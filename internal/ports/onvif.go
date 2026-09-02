package ports

import (
	"context"
	"time"

	"hydrastream/internal/domain"
)

// ONVIFDiscoverer defines the secondary port for scanning local networks and probing ONVIF IP cameras.
type ONVIFDiscoverer interface {
	// Discover broadcasts WS-Discovery probes to find cameras on the local network.
	Discover(ctx context.Context, timeout time.Duration) ([]domain.ONVIFDevice, error)

	// ProbeDevice queries a specific camera via SOAP to extract device info, profiles, and stream URI.
	ProbeDevice(ctx context.Context, ipAddress string, port int, username, password string) (*domain.ONVIFDevice, error)

	// GetStreamURI retrieves the RTSP stream URL for a given profile token.
	GetStreamURI(ctx context.Context, xaddr, username, password, profileToken string) (string, error)
}
