package main

// UPnP IGD minimal : demande au routeur d'ouvrir (puis de refermer) les ports du serveur.

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type igd struct {
	controlURL string
	service    string
	localIP    string
}

func discoverIGD(ctx context.Context) (*igd, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	for _, st := range []string{"urn:schemas-upnp-org:device:InternetGatewayDevice:1", "urn:schemas-upnp-org:device:InternetGatewayDevice:2"} {
		msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + st + "\r\n\r\n"
		conn.WriteToUDP([]byte(msg), dst)
	}
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, 2048)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(deadline)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		loc := ""
		for _, l := range strings.Split(string(buf[:n]), "\r\n") {
			if k, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "location") {
				loc = strings.TrimSpace(v)
			}
		}
		if loc == "" {
			continue
		}
		if g, err := describe(ctx, loc); err == nil {
			return g, nil
		}
	}
	return nil, errors.New("aucun routeur UPnP trouvé")
}

type xService struct {
	Type    string `xml:"serviceType"`
	Control string `xml:"controlURL"`
}
type xDevice struct {
	Services []xService `xml:"serviceList>service"`
	Devices  []xDevice  `xml:"deviceList>device"`
}

func describe(ctx context.Context, loc string) (*igd, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", loc, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var root struct {
		Device xDevice `xml:"device"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&root); err != nil {
		return nil, err
	}
	var find func(d xDevice) *xService
	find = func(d xDevice) *xService {
		for i, s := range d.Services {
			if strings.Contains(s.Type, "WANIPConnection") || strings.Contains(s.Type, "WANPPPConnection") {
				return &d.Services[i]
			}
		}
		for _, c := range d.Devices {
			if s := find(c); s != nil {
				return s
			}
		}
		return nil
	}
	s := find(root.Device)
	if s == nil {
		return nil, errors.New("pas de service WANIPConnection")
	}
	base, _ := url.Parse(loc)
	cu, err := base.Parse(s.Control)
	if err != nil {
		return nil, err
	}
	// adresse locale utilisée pour joindre le routeur
	c, err := net.Dial("udp4", base.Host)
	if err != nil {
		return nil, err
	}
	local := c.LocalAddr().(*net.UDPAddr).IP.String()
	c.Close()
	return &igd{controlURL: cu.String(), service: s.Type, localIP: local}, nil
}

func (g *igd) soap(ctx context.Context, action, args string) error {
	body := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" ` +
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` + action + ` xmlns:u="` + g.service + `">` +
		args + `</u:` + action + `></s:Body></s:Envelope>`
	req, _ := http.NewRequestWithContext(ctx, "POST", g.controlURL, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+g.service+`#`+action+`"`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s : HTTP %d %s", action, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (g *igd) add(ctx context.Context, port int) error {
	return g.soap(ctx, "AddPortMapping", fmt.Sprintf(
		"<NewRemoteHost></NewRemoteHost><NewExternalPort>%d</NewExternalPort><NewProtocol>TCP</NewProtocol>"+
			"<NewInternalPort>%d</NewInternalPort><NewInternalClient>%s</NewInternalClient><NewEnabled>1</NewEnabled>"+
			"<NewPortMappingDescription>pa-coop</NewPortMappingDescription><NewLeaseDuration>0</NewLeaseDuration>",
		port, port, g.localIP))
}

func (g *igd) remove(ctx context.Context, port int) error {
	return g.soap(ctx, "DeletePortMapping", fmt.Sprintf(
		"<NewRemoteHost></NewRemoteHost><NewExternalPort>%d</NewExternalPort><NewProtocol>TCP</NewProtocol>", port))
}
