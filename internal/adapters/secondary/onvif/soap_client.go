package onvif

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hydrastream/internal/domain"
)

type soapClient struct {
	client *http.Client
}

func newSOAPClient() *soapClient {
	return &soapClient{
		client: &http.Client{Timeout: 6 * time.Second},
	}
}

func buildSecurityHeader(username, password string) string {
	if username == "" {
		return ""
	}

	created := time.Now().UTC().Format(time.RFC3339)
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)

	// Password_Digest = Base64 ( SHA-1 ( raw_nonce + created + password ) )
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)

	return fmt.Sprintf(`
  <s:Header>
    <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
                   xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
      <wsse:UsernameToken>
        <wsse:Username>%s</wsse:Username>
        <wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
        <wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
        <wsu:Created>%s</wsu:Created>
      </wsse:UsernameToken>
    </wsse:Security>
  </s:Header>`, username, digest, nonceB64, created)
}

func (s *soapClient) call(ctx context.Context, endpoint, action, username, password, body string) (string, error) {
	header := buildSecurityHeader(username, password)
	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tds="http://www.onvif.org/ver10/device/wsdl"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
%s
  <s:Body>
%s
  </s:Body>
</s:Envelope>`, header, body)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBufferString(envelope))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8; action=\""+action+"\"")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(respBytes), nil
}

func (s *soapClient) getDeviceInformation(ctx context.Context, endpoint, user, pass string) (string, string, string, string, error) {
	resp, err := s.call(ctx, endpoint, "http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation", user, pass, `<tds:GetDeviceInformation/>`)
	if err != nil {
		return "", "", "", "", err
	}

	mfg := extractTag(resp, "Manufacturer")
	model := extractTag(resp, "Model")
	fw := extractTag(resp, "FirmwareVersion")
	serial := extractTag(resp, "SerialNumber")
	return mfg, model, fw, serial, nil
}

func (s *soapClient) getMediaXAddr(ctx context.Context, endpoint, user, pass string) (string, error) {
	resp, err := s.call(ctx, endpoint, "http://www.onvif.org/ver10/device/wsdl/GetCapabilities", user, pass,
		`<tds:GetCapabilities><tds:Category>Media</tds:Category></tds:GetCapabilities>`)
	if err != nil {
		return "", err
	}
	mediaXAddr := extractTag(resp, "XAddr")
	if mediaXAddr != "" {
		return mediaXAddr, nil
	}
	// Fallback to media service on same host
	return strings.Replace(endpoint, "/device_service", "/media_service", 1), nil
}

func (s *soapClient) getProfiles(ctx context.Context, mediaEndpoint, user, pass string) ([]domain.ONVIFProfile, error) {
	resp, err := s.call(ctx, mediaEndpoint, "http://www.onvif.org/ver10/media/wsdl/GetProfiles", user, pass, `<trt:GetProfiles/>`)
	if err != nil {
		return nil, err
	}

	profileRegex := regexp.MustCompile(`(?s)<[^>]*Profiles[^>]*token="([^"]+)"[^>]*>(.*?)</[^>]*Profiles>`)
	matches := profileRegex.FindAllStringSubmatch(resp, -1)

	var profiles []domain.ONVIFProfile
	for _, m := range matches {
		token := m[1]
		body := m[2]
		name := extractTag(body, "Name")
		encoding := extractTag(body, "Encoding")
		if encoding == "" {
			encoding = "H264"
		}
		wStr := extractTag(body, "Width")
		hStr := extractTag(body, "Height")
		fpsStr := extractTag(body, "FrameRateLimit")

		w, _ := strconv.Atoi(wStr)
		h, _ := strconv.Atoi(hStr)
		fps, _ := strconv.Atoi(fpsStr)
		if w == 0 {
			w = 1920
		}
		if h == 0 {
			h = 1080
		}
		if fps == 0 {
			fps = 30
		}

		profiles = append(profiles, domain.ONVIFProfile{
			Token:    token,
			Name:     name,
			Encoding: encoding,
			Width:    w,
			Height:   h,
			FPS:      fps,
		})
	}
	return profiles, nil
}

func (s *soapClient) getStreamURI(ctx context.Context, mediaEndpoint, user, pass, profileToken string) (string, error) {
	body := fmt.Sprintf(`
    <trt:GetStreamUri>
      <trt:StreamSetup>
        <tt:Stream>RTP-Unicast</tt:Stream>
        <tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport>
      </trt:StreamSetup>
      <trt:ProfileToken>%s</trt:ProfileToken>
    </trt:GetStreamUri>`, profileToken)

	resp, err := s.call(ctx, mediaEndpoint, "http://www.onvif.org/ver10/media/wsdl/GetStreamUri", user, pass, body)
	if err != nil {
		return "", err
	}

	rawURI := extractTag(resp, "Uri")
	if rawURI == "" {
		return "", fmt.Errorf("no stream uri returned in SOAP response")
	}

	if user != "" && !strings.Contains(rawURI, "@") {
		u, err := url.Parse(rawURI)
		if err == nil {
			u.User = url.UserPassword(user, pass)
			return u.String(), nil
		}
	}
	return rawURI, nil
}

func extractTag(xmlStr, tagName string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?i)<[^>]*%s[^>]*>([^<]+)</`, tagName))
	m := re.FindStringSubmatch(xmlStr)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
