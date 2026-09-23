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
	"sync"
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

// wsGenericProbeTemplate matches any ONVIF device without type restriction.
const wsGenericProbeTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
            xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
  <e:Header>
    <w:MessageID>uuid:%s</w:MessageID>
    <w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>
    <w:Action a:mustUnderstand="true" xmlns:a="http://www.w3.org/2003/05/soap-envelope">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>
  </e:Header>
  <e:Body>
    <d:Probe/>
  </e:Body>
</e:Envelope>`

type ifaceTarget struct {
	ip        net.IP
	broadcast net.IP
}

func getActiveIPv4Interfaces() []ifaceTarget {
	var targets []ifaceTarget
	ifaces, err := net.Interfaces()
	if err != nil {
		return []ifaceTarget{{ip: net.IPv4zero, broadcast: net.IPv4bcast}}
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			ip4 := ipNet.IP.To4()
			// Skip APIPA 169.254 if possible, unless no other exists
			bcast := make(net.IP, len(ip4))
			for i := range ip4 {
				bcast[i] = ip4[i] | ^ipNet.Mask[i]
			}
			targets = append(targets, ifaceTarget{ip: ip4, broadcast: bcast})
		}
	}

	if len(targets) == 0 {
		targets = append(targets, ifaceTarget{ip: net.IPv4zero, broadcast: net.IPv4bcast})
	}
	return targets
}

// performWSDiscovery sends WS-Discovery probe packets across all active network interfaces.
func performWSDiscovery(ctx context.Context, timeout time.Duration) ([]domain.ONVIFDevice, error) {
	multicastDest, err := net.ResolveUDPAddr("udp4", wsMulticastAddr)
	if err != nil {
		return nil, err
	}
	globalBcast, _ := net.ResolveUDPAddr("udp4", "255.255.255.255:3702")

	targets := getActiveIPv4Interfaces()
	var (
		mu      sync.Mutex
		seen    = make(map[string]bool)
		devices []domain.ONVIFDevice
		wg      sync.WaitGroup
	)

	deadline := time.Now().Add(timeout)

	probeMsg1 := fmt.Sprintf(wsProbeTemplate, generateUUID())
	probeMsg2 := fmt.Sprintf(wsGenericProbeTemplate, generateUUID())

	for _, tgt := range targets {
		wg.Add(1)
		go func(t ifaceTarget) {
			defer wg.Done()

			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: t.ip, Port: 0})
			if err != nil {
				// Fallback to binding 0.0.0.0
				conn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
				if err != nil {
					return
				}
			}
			defer conn.Close()

			_ = conn.SetReadDeadline(deadline)

			// Send to Multicast, Global Broadcast, and Subnet Broadcast
			destinations := []*net.UDPAddr{multicastDest}
			if globalBcast != nil {
				destinations = append(destinations, globalBcast)
			}
			if t.broadcast != nil && !t.broadcast.Equal(net.IPv4bcast) {
				if sb, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:3702", t.broadcast.String())); err == nil {
					destinations = append(destinations, sb)
				}
			}

			for _, dst := range destinations {
				_, _ = conn.WriteTo([]byte(probeMsg1), dst)
				_, _ = conn.WriteTo([]byte(probeMsg2), dst)
			}

			buf := make([]byte, 8192)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if time.Now().After(deadline) {
					break
				}

				n, srcAddr, err := conn.ReadFrom(buf)
				if err != nil {
					break
				}

				rawXML := string(buf[:n])
				dev := parseProbeMatch(rawXML, srcAddr.String())
				if dev != nil {
					mu.Lock()
					key := dev.IPAddress + ":" + dev.XAddr
					if !seen[key] {
						seen[key] = true
						devices = append(devices, *dev)
					}
					mu.Unlock()
				}
			}
		}(tgt)
	}

	wg.Wait()
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
	if ip == "" && srcAddr != "" {
		if host, _, err := net.SplitHostPort(srcAddr); err == nil {
			ip = host
		} else {
			ip = srcAddr
		}
	}
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
