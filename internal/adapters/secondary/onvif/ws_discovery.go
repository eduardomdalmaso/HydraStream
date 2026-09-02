package onvif

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hydrastream/internal/domain"
)

const (
	wsMulticastAddr = "239.255.255.250:3702"
	wsProbeTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
            xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
            xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
  <e:Header>
    <w:MessageID>uuid:%s</w:MessageID>
    <w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>
    <w:Action a:mustUnderstand="true" xmlns:a="http://www.w3.org/2003/05/soap-envelope">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>
  </e:Header>
  <e:Body>
    <d:Probe>
      <d:Types>dn:NetworkVideoTransmitter</d:Types>
    </d:Probe>
  </e:Body>
</e:Envelope>`
)

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// performWSDiscovery sends WS-Discovery probe packets and gathers responses.
func performWSDiscovery(ctx context.Context, timeout time.Duration) ([]domain.ONVIFDevice, error) {
	dest, err := net.ResolveUDPAddr("udp4", wsMulticastAddr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	msg := fmt.Sprintf(wsProbeTemplate, generateUUID())
	if _, err := conn.WriteTo([]byte(msg), dest); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)

	seen := make(map[string]bool)
	var devices []domain.ONVIFDevice

	buf := make([]byte, 8192)
	for {
		select {
		case <-ctx.Done():
			return devices, nil
		default:
		}

		if time.Now().After(deadline) {
			break
		}

		n, srcAddr, err := conn.ReadFrom(buf)
		if err != nil {
			break // Timeout reached
		}

		rawXML := string(buf[:n])
		dev := parseProbeMatch(rawXML, srcAddr.String())
		if dev != nil && !seen[dev.XAddr] {
			seen[dev.XAddr] = true
			devices = append(devices, *dev)
		}
	}

	return devices, nil
}

func parseProbeMatch(xmlData, srcAddr string) *domain.ONVIFDevice {
	xaddrRegex := regexp.MustCompile(`(?i)<[^>]*XAddrs[^>]*>([^<]+)</`)
	match := xaddrRegex.FindStringSubmatch(xmlData)
	if len(match) < 2 {
		return nil
	}

	rawXAddr := strings.TrimSpace(strings.Fields(match[1])[0])
	u, err := url.Parse(rawXAddr)
	if err != nil {
		return nil
	}

	ip := u.Hostname()
	port := 80
	if u.Port() != "" {
		if p, err := strconv.Atoi(u.Port()); err == nil {
			port = p
		}
	} else if u.Scheme == "https" {
		port = 443
	}

	// Extract scope names
	name := "ONVIF Camera"
	model := "Generic IP Camera"
	mfg := "ONVIF"

	scopesRegex := regexp.MustCompile(`(?i)<[^>]*Scopes[^>]*>([^<]+)</`)
	if sm := scopesRegex.FindStringSubmatch(xmlData); len(sm) >= 2 {
		scopes := strings.Fields(sm[1])
		for _, sc := range scopes {
			decoded, _ := url.QueryUnescape(sc)
			if strings.Contains(decoded, "/name/") {
				name = decoded[strings.LastIndex(decoded, "/name/")+6:]
			} else if strings.Contains(decoded, "/hardware/") {
				model = decoded[strings.LastIndex(decoded, "/hardware/")+10:]
			} else if strings.Contains(decoded, "/mfr/") {
				mfg = decoded[strings.LastIndex(decoded, "/mfr/")+5:]
			}
		}
	}

	cleanName := strings.ReplaceAll(name, "_", " ")
	return &domain.ONVIFDevice{
		DeviceID:     fmt.Sprintf("onvif_%s_%d", strings.ReplaceAll(ip, ".", "_"), port),
		Name:         cleanName,
		Manufacturer: mfg,
		Model:        model,
		IPAddress:    ip,
		Port:         port,
		XAddr:        rawXAddr,
		DiscoveredAt: time.Now(),
	}
}
