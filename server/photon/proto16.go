// Package photon implémente la partie du protocole Photon (Protocol16 sur TCP) utilisée par Prison Architect.
package photon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Types Go utilisés pour les valeurs Protocol16 :
//   nil, byte, bool, int16, int32, int64, float32, float64, string, []byte,
//   []int32, []string, []any (tableau d'objets), Array (tableau typé), Hashtable, Dict.

// KV est une entrée de table (l'ordre est conservé).
type KV struct {
	K any
	V any
}

// Hashtable = table Photon ('h').
type Hashtable []KV

// Get renvoie la valeur d'une clé (comparaison simple).
func (h Hashtable) Get(k any) (any, bool) {
	for _, kv := range h {
		if kv.K == k {
			return kv.V, true
		}
	}
	return nil, false
}

// Set remplace ou ajoute une clé.
func (h *Hashtable) Set(k, v any) {
	for i, kv := range *h {
		if kv.K == k {
			(*h)[i].V = v
			return
		}
	}
	*h = append(*h, KV{k, v})
}

// Dict = dictionnaire typé ('D').
type Dict struct {
	KT, VT byte
	Items  []KV
}

// Array = tableau typé ('y').
type Array struct {
	T     byte
	Items []any
}

// Params = paramètres d'une opération ou d'un événement (ordre conservé).
type Params []PKV

// PKV = paramètre.
type PKV struct {
	K byte
	V any
}

func (p Params) Get(k byte) (any, bool) {
	for _, kv := range p {
		if kv.K == k {
			return kv.V, true
		}
	}
	return nil, false
}

func (p *Params) Set(k byte, v any) {
	for i, kv := range *p {
		if kv.K == k {
			(*p)[i].V = v
			return
		}
	}
	*p = append(*p, PKV{k, v})
}

func (p Params) String() string {
	var b bytes.Buffer
	b.WriteString("{")
	for i, kv := range p {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d: %s", kv.K, Show(kv.V))
	}
	b.WriteString("}")
	return b.String()
}

// Show rend une valeur lisible pour les journaux.
func Show(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		if len(x) > 40 {
			return fmt.Sprintf("%q…(%d)", x[:40], len(x))
		}
		return fmt.Sprintf("%q", x)
	case []byte:
		if len(x) > 16 {
			return fmt.Sprintf("bytes[%d]:%x…", len(x), x[:16])
		}
		return fmt.Sprintf("bytes[%d]:%x", len(x), x)
	case Hashtable:
		var b bytes.Buffer
		b.WriteString("h{")
		for i, kv := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(Show(kv.K) + ": " + Show(kv.V))
		}
		b.WriteString("}")
		return b.String()
	case Array:
		return fmt.Sprintf("y%c%v", x.T, x.Items)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// ---------- lecture ----------

type reader struct{ r *bytes.Reader }

func (r reader) u8() (byte, error) { return r.r.ReadByte() }

func (r reader) n(n int) ([]byte, error) {
	if n < 0 || n > r.r.Len() {
		return nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r.r, b)
	return b, err
}

func (r reader) u16() (uint16, error) {
	b, err := r.n(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r reader) i32() (int32, error) {
	b, err := r.n(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b)), nil
}

func (r reader) value() (any, error) {
	t, err := r.u8()
	if err != nil {
		return nil, err
	}
	return r.typed(t)
}

func (r reader) typed(t byte) (any, error) {
	switch t {
	case 0, '*':
		return nil, nil
	case 'b':
		return r.u8()
	case 'o':
		b, err := r.u8()
		return b != 0, err
	case 'k':
		v, err := r.u16()
		return int16(v), err
	case 'i':
		return r.i32()
	case 'l':
		b, err := r.n(8)
		if err != nil {
			return nil, err
		}
		return int64(binary.BigEndian.Uint64(b)), nil
	case 'f':
		b, err := r.n(4)
		if err != nil {
			return nil, err
		}
		return math.Float32frombits(binary.BigEndian.Uint32(b)), nil
	case 'd':
		b, err := r.n(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	case 's':
		n, err := r.u16()
		if err != nil {
			return nil, err
		}
		b, err := r.n(int(n))
		return string(b), err
	case 'x':
		n, err := r.i32()
		if err != nil {
			return nil, err
		}
		return r.n(int(n))
	case 'n':
		n, err := r.i32()
		if err != nil {
			return nil, err
		}
		out := make([]int32, 0, n)
		for i := int32(0); i < n; i++ {
			v, err := r.i32()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 'a':
		n, err := r.u16()
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, n)
		for i := 0; i < int(n); i++ {
			v, err := r.typed('s')
			if err != nil {
				return nil, err
			}
			out = append(out, v.(string))
		}
		return out, nil
	case 'y':
		n, err := r.u16()
		if err != nil {
			return nil, err
		}
		et, err := r.u8()
		if err != nil {
			return nil, err
		}
		a := Array{T: et}
		for i := 0; i < int(n); i++ {
			v, err := r.typed(et)
			if err != nil {
				return nil, err
			}
			a.Items = append(a.Items, v)
		}
		return a, nil
	case 'z':
		n, err := r.u16()
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, n)
		for i := 0; i < int(n); i++ {
			v, err := r.value()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 'h':
		n, err := r.u16()
		if err != nil {
			return nil, err
		}
		h := Hashtable{}
		for i := 0; i < int(n); i++ {
			k, err := r.value()
			if err != nil {
				return nil, err
			}
			v, err := r.value()
			if err != nil {
				return nil, err
			}
			h = append(h, KV{k, v})
		}
		return h, nil
	case 'D':
		kt, err := r.u8()
		if err != nil {
			return nil, err
		}
		vt, err := r.u8()
		if err != nil {
			return nil, err
		}
		n, err := r.u16()
		if err != nil {
			return nil, err
		}
		d := Dict{KT: kt, VT: vt}
		for i := 0; i < int(n); i++ {
			var k, v any
			if kt == 0 || kt == '*' {
				k, err = r.value()
			} else {
				k, err = r.typed(kt)
			}
			if err != nil {
				return nil, err
			}
			if vt == 0 || vt == '*' {
				v, err = r.value()
			} else {
				v, err = r.typed(vt)
			}
			if err != nil {
				return nil, err
			}
			d.Items = append(d.Items, KV{k, v})
		}
		return d, nil
	case 'c':
		ct, err := r.u8()
		if err != nil {
			return nil, err
		}
		n, err := r.u16()
		if err != nil {
			return nil, err
		}
		b, err := r.n(int(n))
		return Custom{ct, b}, err
	}
	return nil, fmt.Errorf("type Protocol16 inconnu %#x", t)
}

// Custom = type personnalisé ('c').
type Custom struct {
	T    byte
	Data []byte
}

func (r reader) params() (Params, error) {
	n, err := r.u16()
	if err != nil {
		return nil, err
	}
	p := make(Params, 0, n)
	for i := 0; i < int(n); i++ {
		k, err := r.u8()
		if err != nil {
			return nil, err
		}
		v, err := r.value()
		if err != nil {
			return nil, err
		}
		p = append(p, PKV{k, v})
	}
	return p, nil
}

// ---------- écriture ----------

type writer struct{ b *bytes.Buffer }

func (w writer) u16(v int) { w.b.Write([]byte{byte(v >> 8), byte(v)}) }
func (w writer) i32(v int32) {
	var t [4]byte
	binary.BigEndian.PutUint32(t[:], uint32(v))
	w.b.Write(t[:])
}

func typeOf(v any) byte {
	switch x := v.(type) {
	case nil:
		return '*'
	case byte:
		return 'b'
	case bool:
		return 'o'
	case int16:
		return 'k'
	case int32:
		return 'i'
	case int:
		return 'i'
	case int64:
		return 'l'
	case float32:
		return 'f'
	case float64:
		return 'd'
	case string:
		return 's'
	case []byte:
		return 'x'
	case []int32:
		return 'n'
	case []string:
		return 'a'
	case Array:
		return 'y'
	case []any:
		return 'z'
	case Hashtable:
		return 'h'
	case Dict:
		return 'D'
	case Custom:
		return 'c'
	default:
		panic(fmt.Sprintf("photon: type non sérialisable %T", x))
	}
}

func (w writer) value(v any) {
	if a, ok := v.([]int32); ok { // le serveur officiel envoie les int[] en tableau typé 'y' 'i'
		w.b.WriteByte('y')
		w.u16(len(a))
		w.b.WriteByte('i')
		for _, x := range a {
			w.i32(x)
		}
		return
	}
	t := typeOf(v)
	w.b.WriteByte(t)
	w.typed(t, v)
}

func (w writer) typed(t byte, v any) {
	switch t {
	case '*', 0:
	case 'b':
		w.b.WriteByte(v.(byte))
	case 'o':
		if v.(bool) {
			w.b.WriteByte(1)
		} else {
			w.b.WriteByte(0)
		}
	case 'k':
		w.u16(int(uint16(v.(int16))))
	case 'i':
		switch x := v.(type) {
		case int32:
			w.i32(x)
		case int:
			w.i32(int32(x))
		}
	case 'l':
		var t8 [8]byte
		binary.BigEndian.PutUint64(t8[:], uint64(v.(int64)))
		w.b.Write(t8[:])
	case 'f':
		var t4 [4]byte
		binary.BigEndian.PutUint32(t4[:], math.Float32bits(v.(float32)))
		w.b.Write(t4[:])
	case 'd':
		var t8 [8]byte
		binary.BigEndian.PutUint64(t8[:], math.Float64bits(v.(float64)))
		w.b.Write(t8[:])
	case 's':
		s := v.(string)
		w.u16(len(s))
		w.b.WriteString(s)
	case 'x':
		b := v.([]byte)
		w.i32(int32(len(b)))
		w.b.Write(b)
	case 'n':
		a := v.([]int32)
		w.i32(int32(len(a)))
		for _, x := range a {
			w.i32(x)
		}
	case 'a':
		a := v.([]string)
		w.u16(len(a))
		for _, s := range a {
			w.typed('s', s)
		}
	case 'y':
		a := v.(Array)
		w.u16(len(a.Items))
		w.b.WriteByte(a.T)
		for _, x := range a.Items {
			w.typed(a.T, x)
		}
	case 'z':
		a := v.([]any)
		w.u16(len(a))
		for _, x := range a {
			w.value(x)
		}
	case 'h':
		h := v.(Hashtable)
		w.u16(len(h))
		for _, kv := range h {
			w.value(kv.K)
			w.value(kv.V)
		}
	case 'D':
		d := v.(Dict)
		w.b.WriteByte(d.KT)
		w.b.WriteByte(d.VT)
		w.u16(len(d.Items))
		for _, kv := range d.Items {
			if d.KT == 0 || d.KT == '*' {
				w.value(kv.K)
			} else {
				w.typed(d.KT, kv.K)
			}
			if d.VT == 0 || d.VT == '*' {
				w.value(kv.V)
			} else {
				w.typed(d.VT, kv.V)
			}
		}
	case 'c':
		c := v.(Custom)
		w.b.WriteByte(c.T)
		w.u16(len(c.Data))
		w.b.Write(c.Data)
	}
}

func (w writer) params(p Params) {
	w.u16(len(p))
	for _, kv := range p {
		w.b.WriteByte(kv.K)
		w.value(kv.V)
	}
}

// ---------- messages ----------

// Types de messages (2e octet après 0xF3, bit 0x80 = chiffré).
const (
	MsgInit             = 0
	MsgInitResponse     = 1
	MsgOpRequest        = 2
	MsgOpResponse       = 3
	MsgEvent            = 4
	MsgInternalRequest  = 6
	MsgInternalResponse = 7
)

// Message = un message Photon décodé.
type Message struct {
	Type      byte // sans le bit de chiffrement
	Encrypted bool
	Code      byte // code d'opération ou d'événement
	Return    int16
	Debug     any
	Params    Params
	Raw       []byte // pour Init
}

var ErrEncrypted = errors.New("message chiffré")

// DecodeBody décode le corps qui suit l'en-tête F3 <type> (déjà déchiffré si besoin).
func DecodeBody(typ byte, body []byte) (*Message, error) {
	m := &Message{Type: typ}
	r := reader{bytes.NewReader(body)}
	var err error
	switch typ {
	case MsgInit:
		m.Raw = body
		return m, nil
	case MsgInitResponse:
		return m, nil
	case MsgOpRequest, MsgInternalRequest:
		if m.Code, err = r.u8(); err != nil {
			return nil, err
		}
		m.Params, err = r.params()
	case MsgOpResponse, MsgInternalResponse:
		if m.Code, err = r.u8(); err != nil {
			return nil, err
		}
		var rc uint16
		if rc, err = r.u16(); err != nil {
			return nil, err
		}
		m.Return = int16(rc)
		if m.Debug, err = r.value(); err != nil {
			return nil, err
		}
		m.Params, err = r.params()
	case MsgEvent:
		if m.Code, err = r.u8(); err != nil {
			return nil, err
		}
		m.Params, err = r.params()
	default:
		return nil, fmt.Errorf("type de message %d non géré", typ)
	}
	return m, err
}

// EncodeBody sérialise le corps (sans F3 <type>).
func (m *Message) EncodeBody() []byte {
	var b bytes.Buffer
	w := writer{&b}
	switch m.Type {
	case MsgOpRequest, MsgInternalRequest:
		b.WriteByte(m.Code)
		w.params(m.Params)
	case MsgOpResponse, MsgInternalResponse:
		b.WriteByte(m.Code)
		w.u16(int(uint16(m.Return)))
		w.value(m.Debug)
		w.params(m.Params)
	case MsgEvent:
		b.WriteByte(m.Code)
		w.params(m.Params)
	case MsgInitResponse:
		b.WriteByte(0)
	case MsgInit:
		b.Write(m.Raw)
	}
	return b.Bytes()
}
