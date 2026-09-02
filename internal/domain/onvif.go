package domain

import "time"

// ONVIFProfile represents a video stream profile exposed by an ONVIF camera.
type ONVIFProfile struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	Encoding   string `json:"encoding"`   // "H264", "H265", "JPEG"
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FPS        int    `json:"fps"`
	RTSPStream string `json:"rtsp_stream"` // Stream URL for this profile
}

// ONVIFDevice represents an IP camera discovered on the local network.
type ONVIFDevice struct {
	DeviceID        string         `json:"device_id"`
	Name            string         `json:"name"`
	Manufacturer    string         `json:"manufacturer"`
	Model           string         `json:"model"`
	FirmwareVersion string         `json:"firmware_version"`
	SerialNumber    string         `json:"serial_number"`
	HardwareID      string         `json:"hardware_id"`
	IPAddress       string         `json:"ip_address"`
	Port            int            `json:"port"`
	XAddr           string         `json:"xaddr"`           // e.g., "http://192.168.1.100:80/onvif/device_service"
	RTSPURL         string         `json:"rtsp_url"`        // Default RTSP stream URL
	Profiles        []ONVIFProfile `json:"profiles"`
	DiscoveredAt    time.Time      `json:"discovered_at"`
}

// ONVIFProbeRequest holds manual IP probe parameters.
type ONVIFProbeRequest struct {
	IPAddress string `json:"ip_address"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}
