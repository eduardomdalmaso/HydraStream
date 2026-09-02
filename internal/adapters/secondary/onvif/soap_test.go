package onvif

import (
	"testing"
)

func TestExtractTag(t *testing.T) {
	xml := `<tds:GetDeviceInformationResponse xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
		<tds:Manufacturer>Hikvision</tds:Manufacturer>
		<tds:Model>DS-2CD2043G2-I</tds:Model>
		<tds:FirmwareVersion>V5.7.1</tds:FirmwareVersion>
		<tds:SerialNumber>DS-2CD2043G2-I20210816AAWR</tds:SerialNumber>
	</tds:GetDeviceInformationResponse>`

	mfg := extractTag(xml, "Manufacturer")
	if mfg != "Hikvision" {
		t.Errorf("expected Hikvision, got %s", mfg)
	}

	model := extractTag(xml, "Model")
	if model != "DS-2CD2043G2-I" {
		t.Errorf("expected DS-2CD2043G2-I, got %s", model)
	}
}

func TestBuildSecurityHeader(t *testing.T) {
	header := buildSecurityHeader("admin", "password123")
	if header == "" {
		t.Errorf("expected non-empty security header")
	}
	if !contains(header, "wsse:UsernameToken") || !contains(header, "admin") {
		t.Errorf("header missing UsernameToken: %s", header)
	}
}

func TestParseProbeMatch(t *testing.T) {
	sample := `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
	<soap:Body>
		<d:ProbeMatches>
			<d:ProbeMatch>
				<d:XAddrs>http://192.168.1.100:80/onvif/device_service</d:XAddrs>
				<d:Scopes>onvif://www.onvif.org/name/Entrance_Cam onvif://www.onvif.org/hardware/DS-2CD onvif://www.onvif.org/mfr/Hikvision</d:Scopes>
			</d:ProbeMatch>
		</d:ProbeMatches>
	</soap:Body>
</soap:Envelope>`

	dev := parseProbeMatch(sample, "192.168.1.100:3702")
	if dev == nil {
		t.Fatalf("failed to parse probe match")
	}
	if dev.IPAddress != "192.168.1.100" || dev.Port != 80 {
		t.Errorf("unexpected IP/Port: %s:%d", dev.IPAddress, dev.Port)
	}
	if dev.Manufacturer != "Hikvision" {
		t.Errorf("expected Hikvision, got %s", dev.Manufacturer)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
