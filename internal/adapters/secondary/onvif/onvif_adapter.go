package onvif

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// Adapter implements the ports.ONVIFDiscoverer secondary adapter.
type Adapter struct {
	soap *soapClient
}

// NewONVIFAdapter instantiates an ONVIF discovery and SOAP management adapter.
func NewONVIFAdapter() ports.ONVIFDiscoverer {
	return &Adapter{
		soap: newSOAPClient(),
	}
}

// Discover broadcasts WS-Discovery probe packets on the local subnet.
func (a *Adapter) Discover(ctx context.Context, timeout time.Duration) ([]domain.ONVIFDevice, error) {
	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}
	return performWSDiscovery(ctx, timeout)
}

// ProbeDevice queries device information, streaming profiles, and RTSP URL for a target camera.
func (a *Adapter) ProbeDevice(ctx context.Context, ipAddress string, port int, username, password string) (*domain.ONVIFDevice, error) {
	if port <= 0 {
		port = 80
	}
	endpoint := fmt.Sprintf("http://%s:%d/onvif/device_service", ipAddress, port)

	mfg, model, fw, serial, err := a.soap.getDeviceInformation(ctx, endpoint, username, password)
	if err != nil {
		// Try alternative ONVIF port 8899, 8000 or 8080 if 80 failed
		if port == 80 {
			for _, altPort := range []int{8899, 8000, 8080} {
				altEndpoint := fmt.Sprintf("http://%s:%d/onvif/device_service", ipAddress, altPort)
				if m, mod, f, s, err2 := a.soap.getDeviceInformation(ctx, altEndpoint, username, password); err2 == nil {
					endpoint = altEndpoint
					port = altPort
					mfg, model, fw, serial = m, mod, f, s
					err = nil
					break
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("failed to connect to ONVIF device service on %s:%d: %w", ipAddress, port, err)
		}
	}

	mediaEndpoint, err := a.soap.getMediaXAddr(ctx, endpoint, username, password)
	if err != nil {
		mediaEndpoint = fmt.Sprintf("http://%s:%d/onvif/media_service", ipAddress, port)
	}

	profiles, _ := a.soap.getProfiles(ctx, mediaEndpoint, username, password)

	var defaultRTSP string
	for i := range profiles {
		uri, err := a.soap.getStreamURI(ctx, mediaEndpoint, username, password, profiles[i].Token)
		if err == nil && uri != "" {
			profiles[i].RTSPStream = uri
			if defaultRTSP == "" {
				defaultRTSP = uri
			}
		}
	}

	if defaultRTSP == "" {
		if username != "" && password != "" {
			defaultRTSP = fmt.Sprintf("rtsp://%s:%s@%s:554/live/ch0", username, password, ipAddress)
		} else {
			defaultRTSP = fmt.Sprintf("rtsp://%s:554/live/ch0", ipAddress)
		}
	}

	cleanIP := strings.ReplaceAll(ipAddress, ".", "_")
	name := fmt.Sprintf("%s %s", mfg, model)
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("ONVIF Cam %s", ipAddress)
	}

	return &domain.ONVIFDevice{
		DeviceID:        fmt.Sprintf("onvif_%s_%d", cleanIP, port),
		Name:            name,
		Manufacturer:    mfg,
		Model:           model,
		FirmwareVersion: fw,
		SerialNumber:    serial,
		IPAddress:       ipAddress,
		Port:            port,
		XAddr:           endpoint,
		RTSPURL:         defaultRTSP,
		Profiles:        profiles,
		DiscoveredAt:    time.Now(),
	}, nil
}

// GetStreamURI retrieves the RTSP stream URL for a given profile token.
func (a *Adapter) GetStreamURI(ctx context.Context, xaddr, username, password, profileToken string) (string, error) {
	return a.soap.getStreamURI(ctx, xaddr, username, password, profileToken)
}
