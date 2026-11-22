// pacoop-server : serveur Photon maison pour le multijoueur de Prison Architect.
//
// Il joue les trois rôles du cloud Photon : serveur de noms (4533), maître (4530) et serveur de jeu (4531).
// Le jeu ne se sert de Photon que comme relais : toute la simulation reste chez l'hôte.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"pacoop/photon"
)

// Codes d'opération, de paramètre et d'événement (Photon LoadBalancing).
const (
	opAuthenticate  = 230
	opJoinLobby     = 229
	opLeaveLobby    = 228
	opCreateGame    = 227
	opJoinGame      = 226
	opJoinRandom    = 225
	opGetRegions    = 220
	opLeave         = 254
	opRaiseEvent    = 253
	opSetProperties = 252
	opGetProperties = 251
	opChangeGroups  = 248

	pRegion       = 210
	pAddress      = 230
	pUserID       = 225
	pSecret       = 221
	pGameList     = 222
	pRoomName     = 255
	pActorNr      = 254
	pActorList    = 252
	pTargetActor  = 253
	pProperties   = 251
	pBroadcast    = 250
	pActorProps   = 249
	pGameProps    = 248
	pCache        = 247
	pReceiverGrp  = 246
	pData         = 245
	pCode         = 244
	pMasterClient = 203
	pJoinMode     = 215
	pAppStatsPeer = 229
	pAppStatsGame = 228
	pAppStatsMast = 227
	pIsInactive   = 233

	evAppStats       = 226
	evGameList       = 230
	evGameListUpdate = 229
	evJoin           = 255
	evLeave          = 254
	evPropsChanged   = 253

	// Clés de propriétés de partie
	gpMaxPlayers    = byte(255)
	gpIsVisible     = byte(254)
	gpIsOpen        = byte(253)
	gpPlayerCount   = byte(252)
	gpRemoved       = byte(251)
	gpLobbyProps    = byte(250)
	gpMaxPlayersInt = byte(243)
	gpMasterClient  = byte(248)

	rcGameDoesNotExist = 32758
	rcGameFull         = 32765
	rcGameClosed       = 32764
	rcGameIDExists     = 32766
)

var (
	public     = flag.String("public", "", "adresse renvoyée aux clients pour joindre le maître et le serveur de jeu ; vide = « pacoop.server », que le mod pa-coop traduit vers le serveur choisi par le joueur (pour un jeu sans mod, mettre l'IP publique)")
	upnp       = flag.Bool("upnp", false, "demander au routeur (UPnP) d'ouvrir les ports TCP du serveur, et les refermer à l'arrêt")
	regionsArg = flag.String("regions", "", "liste de serveurs à proposer dans le menu Region du jeu : nom=hôte[:port_noms],… (port 4533 par défaut)")
	nsOnly     = flag.Bool("ns-only", false, "ne lancer que le serveur de noms (avec -regions : sélecteur de serveurs local, lancé par le mod)")
	upnpProbe  = flag.Bool("upnp-test", false, "cherche seulement le routeur UPnP, l'affiche et quitte (n'ouvre rien)")
	chaosFile  = flag.String("chaos-file", "", "TEST : si ce fichier existe, perdre les messages de l'hôte vers les autres joueurs pendant le nombre de secondes qu'il contient (simule une mauvaise connexion)")
	parentPID  = flag.Int("parent", 0, "s'arrêter quand ce processus (le jeu) se termine")
	nsPort     = flag.Int("ns-port", 4533, "port du serveur de noms")
	masterPort = flag.Int("master-port", 4530, "port du serveur maître")
	gamePort   = flag.Int("game-port", 4531, "port du serveur de jeu")
	region     = flag.String("region", "eu", "nom de région annoncé au jeu")
	maxPlayers = flag.Int("max-players", 16, "nombre maximum de joueurs par partie (le jeu demande 4)")
	verbose    = flag.Bool("v", false, "journal détaillé des opérations")
)

type regionEntry struct{ code, master string }

var regions []regionEntry

// parseRegions lit -regions : « nom=hôte[:port_noms] », le maître étant sur port_noms-3 (4533 -> 4530).
func parseRegions() error {
	if *regionsArg == "" {
		return nil
	}
	for _, it := range strings.Split(*regionsArg, ",") {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		name, hp, ok := strings.Cut(it, "=")
		if !ok {
			return fmt.Errorf("%q : nom=hôte attendu", it)
		}
		host, port := hp, 4533
		if h, p, err := net.SplitHostPort(hp); err == nil {
			host = h
			fmt.Sscanf(p, "%d", &port)
		}
		if strings.EqualFold(host, "local") {
			host = "127.0.0.1"
		}
		code := strings.ToLower(strings.Join(strings.Fields(name), "_"))
		regions = append(regions, regionEntry{code, net.JoinHostPort(host, fmt.Sprint(port-3))})
		log.Printf("région « %s » -> maître %s", code, regions[len(regions)-1].master)
	}
	return nil
}

func pubHost() string {
	if *public != "" {
		return *public
	}
	return "pacoop.server"
}

func main() {
	flag.Parse()
	srv := &server{rooms: map[string]*room{}, lobby: map[*client]bool{}}
	ports := []int{*nsPort, *masterPort, *gamePort}
	if *upnpProbe {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		g, err := discoverIGD(ctx)
		if err != nil {
			log.Fatalf("UPnP : %v", err)
		}
		log.Printf("UPnP : routeur trouvé, service %s, contrôle %s, adresse locale %s", g.service, g.controlURL, g.localIP)
		return
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	var gw *igd
	if *upnp {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		g, err := discoverIGD(ctx)
		if err != nil {
			log.Printf("UPnP : %v (ouvre les ports %v à la main pour jouer par Internet)", err, ports)
		} else {
			for _, p := range ports {
				if err := g.add(ctx, p); err != nil {
					log.Printf("UPnP : port %d : %v", p, err)
				} else {
					log.Printf("UPnP : port %d ouvert vers %s", p, g.localIP)
				}
			}
			gw = g
		}
		cancel()
	}
	if *parentPID > 0 {
		go watchParent(*parentPID, stop)
	}
	if err := parseRegions(); err != nil {
		log.Fatalf("-regions : %v", err)
	}
	go srv.listen("noms", *nsPort, srv.handleNameServer)
	if !*nsOnly {
		go srv.listen("maître", *masterPort, srv.handleMaster)
		go srv.listen("jeu", *gamePort, srv.handleGame)
	}
	go srv.statsLoop()
	<-stop
	if gw != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		for _, p := range ports {
			gw.remove(ctx, p)
		}
		cancel()
		log.Printf("UPnP : ports refermés")
	}
	log.Printf("arrêt")
}

type server struct {
	mu      sync.Mutex
	rooms   map[string]*room
	lobby   map[*client]bool
	peers   atomic.Int32
	nextUID atomic.Int64
}

type client struct {
	c      *photon.Conn
	id     string
	userID string
	name   string
	room   *room
	actor  int32
	props  photon.Hashtable
}

type room struct {
	name     string
	mu       sync.Mutex
	props    photon.Hashtable
	actors   map[int32]*client
	order    []int32
	next     int32
	master   int32
	created  time.Time
	started  bool // au moins un joueur est entré côté serveur de jeu
	lobbyRef bool
}

func (s *server) listen(role string, port int, h func(*client)) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("%s : %v", role, err)
	}
	log.Printf("serveur %s à l'écoute sur :%d", role, port)
	for {
		nc, err := ln.Accept()
		if err != nil {
			log.Printf("%s accept : %v", role, err)
			continue
		}
		if tc, ok := nc.(*net.TCPConn); ok {
			tc.SetNoDelay(true)
			tc.SetKeepAlive(true)
		}
		go func() {
			cl := &client{c: photon.NewConn(nc), id: nc.RemoteAddr().String()}
			s.peers.Add(1)
			defer s.peers.Add(-1)
			defer nc.Close()
			h(cl)
		}()
	}
}

// handshake gère Init et l'échange de clés ; renvoie le premier message applicatif.
func (s *server) next(cl *client) (*photon.Message, error) {
	for {
		m, err := cl.c.Read()
		if err != nil {
			return nil, err
		}
		switch m.Type {
		case photon.MsgInit:
			if err := cl.c.Write(&photon.Message{Type: photon.MsgInitResponse}, false); err != nil {
				return nil, err
			}
			continue
		case photon.MsgInternalRequest:
			if m.Code == 0 {
				v, _ := m.Params.Get(1)
				pub, _ := v.([]byte)
				out, err := cl.c.KeyExchange(pub)
				if err != nil {
					return nil, err
				}
				if err := cl.c.Write(&photon.Message{Type: photon.MsgInternalResponse, Code: 0,
					Params: photon.Params{{1, out}}}, false); err != nil {
					return nil, err
				}
				continue
			}
			if *verbose {
				log.Printf("[%s] requête interne %d ignorée %v", cl.id, m.Code, m.Params)
			}
			continue
		}
		return m, nil
	}
}

func reply(cl *client, req *photon.Message, rc int16, p photon.Params) error {
	return cl.c.Write(&photon.Message{Type: photon.MsgOpResponse, Code: req.Code, Return: rc, Params: p}, req.Encrypted)
}

// strArray = tableau typé de chaînes ('y' + 's') : le client C++ de Photon ne connaît pas le type 'a'.
func strArray(v ...string) photon.Array {
	a := photon.Array{T: 's'}
	for _, x := range v {
		a.Items = append(a.Items, x)
	}
	return a
}

func token() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func logEnd(role string, cl *client, err error) {
	if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "use of closed") &&
		!strings.Contains(err.Error(), "connection reset") {
		log.Printf("[%s %s] fin : %v", role, cl.id, err)
	} else if *verbose {
		log.Printf("[%s %s] déconnecté", role, cl.id)
	}
}

// ---------------- serveur de noms ----------------

func (s *server) handleNameServer(cl *client) {
	for {
		m, err := s.next(cl)
		if err != nil {
			logEnd("noms", cl, err)
			return
		}
		if *verbose {
			log.Printf("[noms %s] op %d %v (chiffré=%v)", cl.id, m.Code, m.Params, m.Encrypted)
		}
		master := fmt.Sprintf("%s:%d", pubHost(), *masterPort)
		codes, addrs := strArray(*region), strArray(master)
		if len(regions) > 0 {
			codes, addrs = photon.Array{T: 's'}, photon.Array{T: 's'}
			for _, r := range regions {
				codes.Items = append(codes.Items, r.code)
				addrs.Items = append(addrs.Items, r.master)
			}
		}
		switch m.Code {
		case opGetRegions:
			err = reply(cl, m, 0, photon.Params{{pRegion, codes}, {pAddress, addrs}})
		case opAuthenticate, 231: // 231 = AuthenticateOnce
			if len(regions) > 0 {
				master = regions[0].master
				if v, ok := m.Params.Get(pRegion); ok {
					for _, r := range regions {
						if rv, _ := v.(string); strings.EqualFold(rv, r.code) {
							master = r.master
						}
					}
				}
				log.Printf("[noms] serveur choisi : %s", master)
			}
			uid := fmt.Sprintf("u%d", s.nextUID.Add(1))
			err = reply(cl, m, 0, photon.Params{{pAddress, master}, {pUserID, uid}, {pSecret, token()}})
		default:
			log.Printf("[noms %s] opération %d inconnue %v", cl.id, m.Code, m.Params)
			err = reply(cl, m, -2, nil)
		}
		if err != nil {
			logEnd("noms", cl, err)
			return
		}
	}
}

// ---------------- serveur maître ----------------

func (s *server) lobbyEntry(r *room) photon.Hashtable {
	h := photon.Hashtable{}
	listed := map[string]bool{}
	if v, ok := r.props.Get(gpLobbyProps); ok {
		switch x := v.(type) {
		case []string:
			for _, k := range x {
				listed[k] = true
			}
		case []any:
			for _, k := range x {
				if ks, ok := k.(string); ok {
					listed[ks] = true
				}
			}
		case photon.Array:
			for _, k := range x.Items {
				if ks, ok := k.(string); ok {
					listed[ks] = true
				}
			}
		}
	}
	for _, kv := range r.props {
		if ks, ok := kv.K.(string); ok && listed[ks] {
			h = append(h, kv)
		}
	}
	h.Set(gpPlayerCount, byte(len(r.actors)))
	h.Set(gpMaxPlayersInt, int32(*maxPlayers))
	h.Set(gpIsOpen, true)
	h.Set(gpMaxPlayers, byte(min(*maxPlayers, 255)))
	return h
}

func visible(r *room) bool {
	if v, ok := r.props.Get(gpIsVisible); ok {
		if b, ok := v.(bool); ok && !b {
			return false
		}
	}
	return r.started && len(r.actors) > 0
}

func (s *server) gameList() photon.Hashtable {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.rooms))
	for n := range s.rooms {
		names = append(names, n)
	}
	sort.Strings(names)
	h := photon.Hashtable{}
	for _, n := range names {
		r := s.rooms[n]
		r.mu.Lock()
		if visible(r) {
			h = append(h, photon.KV{K: n, V: s.lobbyEntry(r)})
		}
		r.mu.Unlock()
	}
	return h
}

func (s *server) appStats() *photon.Message {
	s.mu.Lock()
	games := len(s.rooms)
	s.mu.Unlock()
	return &photon.Message{Type: photon.MsgEvent, Code: evAppStats, Params: photon.Params{
		{pAppStatsMast, int32(s.peers.Load())}, {pAppStatsPeer, int32(s.peers.Load())}, {pAppStatsGame, int32(games)}}}
}

// pushLobby envoie la liste des parties à jour aux clients du salon.
func (s *server) pushLobby() {
	list := s.gameList()
	s.mu.Lock()
	cls := make([]*client, 0, len(s.lobby))
	for c := range s.lobby {
		cls = append(cls, c)
	}
	s.mu.Unlock()
	ev := &photon.Message{Type: photon.MsgEvent, Code: evGameList, Params: photon.Params{{pGameList, list}}}
	for _, c := range cls {
		c.c.Write(ev, false)
	}
}

func (s *server) statsLoop() {
	for range time.Tick(5 * time.Second) {
		s.mu.Lock()
		cls := make([]*client, 0, len(s.lobby))
		for c := range s.lobby {
			cls = append(cls, c)
		}
		// les parties créées au maître mais jamais ouvertes au serveur de jeu disparaissent
		for n, r := range s.rooms {
			r.mu.Lock()
			if len(r.actors) == 0 && time.Since(r.created) > 2*time.Minute {
				delete(s.rooms, n)
			}
			r.mu.Unlock()
		}
		s.mu.Unlock()
		st := s.appStats()
		for _, c := range cls {
			c.c.Write(st, false)
		}
	}
}

func (s *server) handleMaster(cl *client) {
	defer func() {
		s.mu.Lock()
		delete(s.lobby, cl)
		s.mu.Unlock()
	}()
	gameAddr := fmt.Sprintf("%s:%d", pubHost(), *gamePort)
	for {
		m, err := s.next(cl)
		if err != nil {
			logEnd("maître", cl, err)
			return
		}
		if *verbose {
			log.Printf("[maître %s] op %d %v", cl.id, m.Code, m.Params)
		}
		switch m.Code {
		case opAuthenticate:
			cl.userID = fmt.Sprintf("u%d", s.nextUID.Add(1))
			err = reply(cl, m, 0, photon.Params{{pUserID, cl.userID}, {pSecret, token()}})
			if err == nil {
				err = cl.c.Write(s.appStats(), false)
			}
		case opJoinLobby:
			s.mu.Lock()
			s.lobby[cl] = true
			s.mu.Unlock()
			if err = reply(cl, m, 0, nil); err == nil {
				err = cl.c.Write(&photon.Message{Type: photon.MsgEvent, Code: evGameList,
					Params: photon.Params{{pGameList, s.gameList()}}}, false)
			}
		case opLeaveLobby:
			s.mu.Lock()
			delete(s.lobby, cl)
			s.mu.Unlock()
			err = reply(cl, m, 0, nil)
		case opCreateGame:
			name, _ := m.Params.Get(pRoomName)
			n, _ := name.(string)
			if n == "" {
				n = token()[:12]
			}
			s.mu.Lock()
			if r, ok := s.rooms[n]; ok && len(r.actors) > 0 {
				s.mu.Unlock()
				err = reply(cl, m, rcGameIDExists, nil)
				break
			}
			r := &room{name: n, actors: map[int32]*client{}, created: time.Now()}
			if gp, ok := m.Params.Get(pGameProps); ok {
				r.props, _ = gp.(photon.Hashtable)
			}
			s.rooms[n] = r
			s.mu.Unlock()
			log.Printf("partie « %s » créée par %s", n, cl.id)
			err = reply(cl, m, 0, photon.Params{{pAddress, gameAddr}, {pSecret, token()}, {pRoomName, n}})
		case opJoinGame:
			name, _ := m.Params.Get(pRoomName)
			n, _ := name.(string)
			s.mu.Lock()
			r, ok := s.rooms[n]
			s.mu.Unlock()
			if !ok {
				err = reply(cl, m, rcGameDoesNotExist, nil)
				break
			}
			r.mu.Lock()
			full := len(r.actors) >= *maxPlayers
			r.mu.Unlock()
			if full {
				err = reply(cl, m, rcGameFull, nil)
				break
			}
			err = reply(cl, m, 0, photon.Params{{pAddress, gameAddr}, {pSecret, token()}, {pRoomName, n}})
		case opJoinRandom:
			err = reply(cl, m, 32760, nil) // NoRandomMatchFound
		default:
			log.Printf("[maître %s] opération %d non gérée %v", cl.id, m.Code, m.Params)
			err = reply(cl, m, -2, nil)
		}
		if err != nil {
			logEnd("maître", cl, err)
			return
		}
	}
}

// ---------------- serveur de jeu ----------------

func (r *room) actorList() []int32 {
	out := make([]int32, len(r.order))
	copy(out, r.order)
	return out
}

func (r *room) allActorProps() photon.Hashtable {
	h := photon.Hashtable{}
	for _, a := range r.order {
		h = append(h, photon.KV{K: a, V: r.actors[a].props})
	}
	return h
}

func (r *room) others(except int32) []*client {
	out := make([]*client, 0, len(r.order))
	for _, a := range r.order {
		if a != except {
			out = append(out, r.actors[a])
		}
	}
	return out
}

func (s *server) handleGame(cl *client) {
	defer s.leaveRoom(cl)
	for {
		m, err := s.next(cl)
		if err != nil {
			logEnd("jeu", cl, err)
			return
		}
		if *verbose && m.Code != opRaiseEvent {
			log.Printf("[jeu %s] op %d %v", cl.id, m.Code, m.Params)
		}
		switch m.Code {
		case opAuthenticate:
			err = reply(cl, m, 0, nil)
		case opCreateGame, opJoinGame:
			err = s.enterRoom(cl, m)
		case opRaiseEvent:
			s.raise(cl, m)
		case opSetProperties:
			err = s.setProps(cl, m)
		case opGetProperties:
			err = reply(cl, m, 0, nil)
		case opChangeGroups:
		case opLeave:
			s.leaveRoom(cl)
			err = reply(cl, m, 0, nil)
		default:
			log.Printf("[jeu %s] opération %d non gérée %v", cl.id, m.Code, m.Params)
			err = reply(cl, m, -2, nil)
		}
		if err != nil {
			logEnd("jeu", cl, err)
			return
		}
	}
}

func (s *server) enterRoom(cl *client, m *photon.Message) error {
	name, _ := m.Params.Get(pRoomName)
	n, _ := name.(string)
	s.mu.Lock()
	r, ok := s.rooms[n]
	if !ok && m.Code == opCreateGame {
		r = &room{name: n, actors: map[int32]*client{}, created: time.Now()}
		s.rooms[n] = r
		ok = true
	}
	s.mu.Unlock()
	if !ok {
		return reply(cl, m, rcGameDoesNotExist, nil)
	}
	r.mu.Lock()
	if len(r.actors) >= *maxPlayers {
		r.mu.Unlock()
		return reply(cl, m, rcGameFull, nil)
	}
	if m.Code == opCreateGame {
		if gp, ok := m.Params.Get(pGameProps); ok {
			if h, ok := gp.(photon.Hashtable); ok {
				r.props = h
			}
		}
	}
	r.next++
	cl.actor = r.next
	cl.room = r
	cl.props = photon.Hashtable{}
	if ap, ok := m.Params.Get(pActorProps); ok {
		if h, ok := ap.(photon.Hashtable); ok {
			cl.props = h
			if v, ok := h.Get(byte(255)); ok {
				cl.name, _ = v.(string)
			}
		}
	}
	r.actors[cl.actor] = cl
	r.order = append(r.order, cl.actor)
	if r.master == 0 {
		r.master = cl.actor
	}
	r.started = true
	props := append(photon.Hashtable{}, r.props...)
	props.Set(gpMasterClient, r.master) // l'hôte (master client) est une propriété de la partie
	resp := photon.Params{
		{pActorNr, cl.actor},
		{pGameProps, props},
		{pActorProps, r.allActorProps()},
		{pActorList, r.actorList()},
		{pMasterClient, r.master},
	}
	join := &photon.Message{Type: photon.MsgEvent, Code: evJoin, Params: photon.Params{
		{pActorNr, cl.actor}, {pActorProps, cl.props}, {pActorList, r.actorList()}}}
	everyone := r.others(0)
	r.mu.Unlock()
	log.Printf("« %s » : %s entre (acteur %d, %q), %d joueur(s)", r.name, cl.id, cl.actor, cl.name, len(everyone))
	if err := reply(cl, m, 0, resp); err != nil {
		return err
	}
	for _, o := range everyone {
		o.c.Write(join, false)
	}
	go s.pushLobby()
	return nil
}

func (s *server) leaveRoom(cl *client) {
	r := cl.room
	if r == nil {
		return
	}
	cl.room = nil
	r.mu.Lock()
	delete(r.actors, cl.actor)
	for i, a := range r.order {
		if a == cl.actor {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	ev := &photon.Message{Type: photon.MsgEvent, Code: evLeave, Params: photon.Params{
		{pActorList, r.actorList()}, {pIsInactive, false}, {pActorNr, cl.actor}}}
	if r.master == cl.actor {
		r.master = 0
		if len(r.order) > 0 {
			r.master = r.order[0]
		}
		ev.Params.Set(pMasterClient, r.master)
	}
	rest := r.others(0)
	empty := len(r.actors) == 0
	r.mu.Unlock()
	log.Printf("« %s » : acteur %d (%q) parti, %d restant(s)", r.name, cl.actor, cl.name, len(rest))
	for _, o := range rest {
		o.c.Write(ev, false)
	}
	if empty {
		s.mu.Lock()
		if s.rooms[r.name] == r {
			delete(s.rooms, r.name)
		}
		s.mu.Unlock()
		log.Printf("« %s » fermée", r.name)
	}
	go s.pushLobby()
}

var chaosUntil atomic.Int64

// chaosActive : mode test « mauvaise connexion », piloté par un fichier.
func chaosActive() bool {
	if *chaosFile == "" {
		return false
	}
	now := time.Now().UnixNano()
	if b, err := os.ReadFile(*chaosFile); err == nil {
		os.Remove(*chaosFile)
		secs := 5
		fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &secs)
		chaosUntil.Store(now + int64(secs)*int64(time.Second))
		log.Printf("TEST : perte des messages de l'hôte pendant %d s", secs)
	}
	return now < chaosUntil.Load()
}

var chaosDropped atomic.Int64

func (s *server) raise(cl *client, m *photon.Message) {
	r := cl.room
	if r == nil {
		return
	}
	code, _ := m.Params.Get(pCode)
	cb, _ := code.(byte)
	// Mode test : on perd les messages de jeu de l'hôte (pas ceux du transfert de la prison, codes 1-8).
	if cl.actor == r.master && cb > 8 && chaosActive() {
		if n := chaosDropped.Add(1); n&(n-1) == 0 {
			log.Printf("TEST : %d message(s) perdu(s)", n)
		}
		return
	}
	data, _ := m.Params.Get(pData)
	if data == nil {
		data = []byte{}
	}
	ev := &photon.Message{Type: photon.MsgEvent, Code: cb, Params: photon.Params{{pData, data}, {pActorNr, cl.actor}}}
	r.mu.Lock()
	var targets []*client
	if t, ok := m.Params.Get(pActorList); ok {
		for _, id := range int32s(t) {
			if c := r.actors[id]; c != nil {
				targets = append(targets, c)
			}
		}
	} else {
		grp := byte(0)
		if g, ok := m.Params.Get(pReceiverGrp); ok {
			grp, _ = g.(byte)
		}
		switch grp {
		case 1: // tous
			targets = r.others(0)
		case 2: // maître
			if c := r.actors[r.master]; c != nil {
				targets = []*client{c}
			}
		default: // les autres
			targets = r.others(cl.actor)
		}
	}
	r.mu.Unlock()
	if *verbose {
		k := fmt.Sprintf("%d>%d", cl.actor, cb)
		n := evCount.add(k)
		if n&(n-1) == 0 { // 1, 2, 4, 8… : journal clairsemé
			ids := make([]int32, 0, len(targets))
			for _, t := range targets {
				ids = append(ids, t.actor)
			}
			d, _ := data.([]byte)
			log.Printf("« %s » événement %d de l'acteur %d vers %v (%d o, n°%d)", r.name, cb, cl.actor, ids, len(d), n)
		}
	}
	for _, t := range targets {
		t.c.Write(ev, false)
	}
}

// int32s accepte une liste d'acteurs sous ses différentes formes Protocol16.
func int32s(v any) []int32 {
	switch x := v.(type) {
	case []int32:
		return x
	case photon.Array:
		out := make([]int32, 0, len(x.Items))
		for _, it := range x.Items {
			if n, ok := it.(int32); ok {
				out = append(out, n)
			}
		}
		return out
	case []any:
		out := make([]int32, 0, len(x))
		for _, it := range x {
			if n, ok := it.(int32); ok {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

type counter struct {
	mu sync.Mutex
	m  map[string]int
}

func (c *counter) add(k string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]int{}
	}
	c.m[k]++
	return c.m[k]
}

var evCount counter

func (s *server) setProps(cl *client, m *photon.Message) error {
	r := cl.room
	if r == nil {
		return reply(cl, m, 0, nil)
	}
	pv, _ := m.Params.Get(pProperties)
	props, _ := pv.(photon.Hashtable)
	var target int32
	if t, ok := m.Params.Get(pActorNr); ok {
		target, _ = t.(int32)
	}
	r.mu.Lock()
	if target != 0 {
		if a := r.actors[target]; a != nil {
			for _, kv := range props {
				a.props.Set(kv.K, kv.V)
			}
		}
	} else {
		for _, kv := range props {
			r.props.Set(kv.K, kv.V)
		}
	}
	rest := r.others(cl.actor)
	r.mu.Unlock()
	if err := reply(cl, m, 0, nil); err != nil {
		return err
	}
	bc := false
	if b, ok := m.Params.Get(pBroadcast); ok {
		bc, _ = b.(bool)
	}
	if bc {
		ev := &photon.Message{Type: photon.MsgEvent, Code: evPropsChanged, Params: photon.Params{
			{pTargetActor, target}, {pProperties, props}, {pActorNr, cl.actor}}}
		for _, o := range rest {
			o.c.Write(ev, false)
		}
	}
	if target == 0 {
		go s.pushLobby()
	}
	return nil
}
