// pacoop-bot : faux joueurs pour tester une partie pa-coop à beaucoup de joueurs.
//
// Chaque bot rejoint la partie comme un vrai client : il demande la prison à l'hôte, en accuse
// réception morceau par morceau, puis reste connecté et donne régulièrement son ping.
//
//	pacoop-bot -server 127.0.0.1:14531 -room ironfort -n 8
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"pacoop/photon"
)

var (
	addr     = flag.String("server", "127.0.0.1:4531", "serveur de jeu pa-coop (hôte:port)")
	roomName = flag.String("room", "", "nom de la partie à rejoindre")
	password = flag.String("password", "", "mot de passe de la partie")
	count    = flag.Int("n", 1, "nombre de bots")
	duration = flag.Duration("for", 10*time.Minute, "durée de présence")
	prefix   = flag.String("name", "Bot", "préfixe des noms")
	names    = flag.String("names", "", "noms des bots, séparés par des virgules (sinon préfixe + numéro)")
)

// Init envoyé par le jeu (version du protocole + AppId), relevé dans une capture.
var initBody, _ = hex.DecodeString("010611410b00003666383639383736626662633439316538666666343231306339363666313435")

const (
	evAuthorise = 5 // client -> hôte : demande de connexion (mot de passe)
	evChunk     = 3 // hôte -> client : morceau de la prison
	evChunkAck  = 2 // client -> hôte : morceau reçu
	evNumChunks = 1 // hôte -> client : nombre de morceaux à venir
)

func gameString(s string) []byte { return append([]byte{0x12, byte(len(s))}, s...) }

func runBot(i int, wg *sync.WaitGroup) {
	defer wg.Done()
	name := fmt.Sprintf("%s%d", *prefix, i)
	if *names != "" {
		if l := strings.Split(*names, ","); i-1 < len(l) {
			name = strings.TrimSpace(l[i-1])
		}
	}
	c, err := photon.Dial(*addr, initBody)
	if err != nil {
		log.Printf("%s : connexion : %v", name, err)
		return
	}
	defer c.Close()
	send := func(code byte, p photon.Params) error {
		return c.Write(&photon.Message{Type: photon.MsgOpRequest, Code: code, Params: p}, false)
	}
	send(230, photon.Params{{221, "bot"}})
	send(226, photon.Params{{255, *roomName}, {250, true},
		{249, photon.Hashtable{{K: byte(255), V: name}, {K: "P", V: int32(40)}}}})
	host := int32(1)
	actor := int32(0)
	chunks, start := 0, time.Now()
	deadline := time.Now().Add(*duration)
	lastPing := time.Now()
	for time.Now().Before(deadline) {
		m, err := c.Read()
		if err != nil {
			log.Printf("%s : déconnecté : %v", name, err)
			return
		}
		switch {
		case m.Type == photon.MsgOpResponse && m.Code == 226:
			if m.Return != 0 {
				log.Printf("%s : refusé (code %d)", name, m.Return)
				return
			}
			if v, ok := m.Params.Get(254); ok {
				actor, _ = v.(int32)
			}
			if v, ok := m.Params.Get(203); ok {
				host, _ = v.(int32)
			}
			log.Printf("%s : dans la partie (acteur %d), demande la prison à l'hôte %d", name, actor, host)
			send(252, photon.Params{{251, photon.Hashtable{{K: "C", V: fmt.Sprintf("0x%06xff", rand.Intn(0xffffff))}}}, {254, actor}, {250, true}})
			send(253, photon.Params{{245, gameString(*password)}, {244, byte(evAuthorise)}, {252, []int32{host}}})
		case m.Type == photon.MsgEvent && m.Code == evNumChunks:
			start = time.Now()
		case m.Type == photon.MsgEvent && m.Code == evChunk:
			chunks++
			send(253, photon.Params{{245, []byte{}}, {244, byte(evChunkAck)}, {252, []int32{host}}})
			if d, ok := m.Params.Get(245); ok {
				if b, ok := d.([]byte); ok && len(b) < 32770 {
					log.Printf("%s : prison reçue (%d morceaux en %.1f s)", name, chunks, time.Since(start).Seconds())
				}
			}
		case m.Type == photon.MsgEvent && m.Code == 254:
			if v, ok := m.Params.Get(254); ok && v.(int32) == host {
				log.Printf("%s : l'hôte est parti", name)
				return
			}
		}
		if time.Since(lastPing) > 4*time.Second {
			lastPing = time.Now()
			send(252, photon.Params{{251, photon.Hashtable{{K: "P", V: int32(35 + rand.Intn(20))}}}, {254, actor}, {250, true}})
		}
	}
	log.Printf("%s : fin (%v)", name, *duration)
}

func main() {
	flag.Parse()
	if *roomName == "" {
		log.Fatal("-room est obligatoire")
	}
	var wg sync.WaitGroup
	for i := 1; i <= *count; i++ {
		wg.Add(1)
		go runBot(i, &wg)
		time.Sleep(3 * time.Second) // l'hôte envoie la prison à un joueur à la fois
	}
	wg.Wait()
}
