package photon

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"
)

// Groupe Oakley 1 (RFC 2409, 768 bits), générateur 22 : l'échange de clés de Photon.
var oakley768, _ = new(big.Int).SetString(
	"FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74020BBEA63B139B22514A08798E3404DD"+
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245E485B576625E7EC6F44C42E9A63A3620FFFFFFFFFFFFFFFF", 16)
var dhGen = big.NewInt(22)

// Conn = une connexion Photon TCP côté serveur.
type Conn struct {
	nc     net.Conn
	br     *bufio.Reader
	wmu    sync.Mutex
	start  time.Time
	aes    cipher.Block // nil tant que l'échange de clés n'est pas fait
	Init   []byte       // corps du message Init du client (version, AppId)
	client bool         // connexion côté client (bots de test)
}

// Dial ouvre une connexion côté client (bots de test) et envoie le message Init.
func Dial(addr string, init []byte) (*Conn, error) {
	nc, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	c := NewConn(nc)
	c.client = true
	if err := c.Write(&Message{Type: MsgInit, Raw: init}, false); err != nil {
		nc.Close()
		return nil, err
	}
	return c, nil
}

func NewConn(nc net.Conn) *Conn {
	return &Conn{nc: nc, br: bufio.NewReaderSize(nc, 64<<10), start: time.Now()}
}

func (c *Conn) RemoteAddr() net.Addr { return c.nc.RemoteAddr() }
func (c *Conn) Close() error         { return c.nc.Close() }

const maxFrame = 64 << 20

// Read lit le prochain message (les pings sont traités ici).
func (c *Conn) Read() (*Message, error) {
	for {
		h, err := c.br.ReadByte()
		if err != nil {
			return nil, err
		}
		switch h {
		case 0xF0: // ping : F0 <temps client 4 o> -> F0 <temps serveur> <temps client>
			if c.client { // côté client : réponse du serveur (8 octets), ignorée
				var t [8]byte
				if _, err := io.ReadFull(c.br, t[:]); err != nil {
					return nil, err
				}
				continue
			}
			var t [4]byte
			if _, err := io.ReadFull(c.br, t[:]); err != nil {
				return nil, err
			}
			var out [9]byte
			out[0] = 0xF0
			binary.BigEndian.PutUint32(out[1:], uint32(time.Since(c.start).Milliseconds()))
			copy(out[5:], t[:])
			c.wmu.Lock()
			_, err = c.nc.Write(out[:])
			c.wmu.Unlock()
			if err != nil {
				return nil, err
			}
			continue
		case 0xFB:
		default:
			return nil, fmt.Errorf("octet d'en-tête inattendu %#x", h)
		}
		var hdr [6]byte
		if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint32(hdr[:4]))
		if n < 9 || n > maxFrame {
			return nil, fmt.Errorf("longueur de trame invalide %d", n)
		}
		body := make([]byte, n-7)
		if _, err := io.ReadFull(c.br, body); err != nil {
			return nil, err
		}
		if body[0] != 0xF3 {
			return nil, fmt.Errorf("magie %#x au lieu de 0xF3", body[0])
		}
		typ := body[1] & 0x7F
		enc := body[1]&0x80 != 0
		payload := body[2:]
		if enc {
			if payload, err = c.decrypt(payload); err != nil {
				return nil, err
			}
		}
		m, err := DecodeBody(typ, payload)
		if err != nil {
			return nil, err
		}
		m.Encrypted = enc
		if typ == MsgInit {
			c.Init = m.Raw
		}
		return m, nil
	}
}

// Write envoie un message (fiable, canal 0), chiffré si demandé et possible.
func (c *Conn) Write(m *Message, encrypt bool) error {
	body := m.EncodeBody()
	typ := m.Type
	if encrypt && c.aes != nil {
		body = c.encrypt(body)
		typ |= 0x80
	}
	reliable := byte(1)
	n := 7 + 2 + len(body)
	frame := make([]byte, 0, n)
	frame = append(frame, 0xFB, byte(n>>24), byte(n>>16), byte(n>>8), byte(n), 0, reliable, 0xF3, typ)
	frame = append(frame, body...)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.nc.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := c.nc.Write(frame)
	return err
}

// KeyExchange répond à la requête interne 0 (clé publique DH du client, 96 octets).
func (c *Conn) KeyExchange(clientPub []byte) ([]byte, error) {
	priv, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 160))
	if err != nil {
		return nil, err
	}
	pub := new(big.Int).Exp(dhGen, priv, oakley768)
	other := new(big.Int).SetBytes(clientPub)
	shared := new(big.Int).Exp(other, priv, oakley768)
	key := sha256.Sum256(shared.Bytes())
	if c.aes, err = aes.NewCipher(key[:]); err != nil {
		return nil, err
	}
	out := pub.Bytes()
	if len(out) < 96 {
		out = append(make([]byte, 96-len(out)), out...)
	}
	return out, nil
}

var zeroIV = make([]byte, 16)

func (c *Conn) decrypt(b []byte) ([]byte, error) {
	if c.aes == nil {
		return nil, ErrEncrypted
	}
	if len(b) == 0 || len(b)%16 != 0 {
		return nil, errors.New("taille chiffrée invalide")
	}
	out := make([]byte, len(b))
	cipher.NewCBCDecrypter(c.aes, zeroIV).CryptBlocks(out, b)
	p := int(out[len(out)-1])
	if p == 0 || p > 16 || p > len(out) {
		return nil, errors.New("bourrage PKCS7 invalide (clé fausse ?)")
	}
	return out[:len(out)-p], nil
}

func (c *Conn) encrypt(b []byte) []byte {
	p := 16 - len(b)%16
	in := make([]byte, len(b)+p)
	copy(in, b)
	for i := len(b); i < len(in); i++ {
		in[i] = byte(p)
	}
	out := make([]byte, len(in))
	cipher.NewCBCEncrypter(c.aes, zeroIV).CryptBlocks(out, in)
	return out
}
