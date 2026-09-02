package domain

import (
	"testing"
	"time"
)

func TestONVIFDeviceCreation(t *testing.T) {
	dev := ONVIFDevice{
		DeviceID:     "cam_01",
		Name:         "Entrance Camera",
		Manufacturer: "Hikvision",
		Model:        "DS-2CD2043G2-I",
		IPAddress:    "192.168.1.64",
		Port:         80,
		XAddr:        "http://192.168.1.64:80/onvif/device_service",
		RTSPURL:      "rtsp://admin:12345@192.168.1.64:554/Streaming/Channels/101",
		DiscoveredAt: time.Now(),
		Profiles: []ONVIFProfile{
			{Token: "Profile_1", Name: "MainStream", Encoding: "H264", Width: 1920, Height: 1080, FPS: 30},
		},
	}

	if dev.Manufacturer != "Hikvision" {
		t.Errorf("expected Hikvision, got %s", dev.Manufacturer)
	}
	if len(dev.Profiles) != 1 || dev.Profiles[0].Width != 1920 {
		t.Errorf("profile width mismatch: %v", dev.Profiles)
	}
}
