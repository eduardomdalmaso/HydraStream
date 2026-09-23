package onvif

import (
	"bytes"
	"context"
	"crypto/md5"
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

func (s *soapClient) getSnapshotURI(ctx context.Context, mediaEndpoint, user, pass, profileToken string) (string, error) {
	body := fmt.Sprintf(`
    <trt:GetSnapshotUri>
      <trt:ProfileToken>%s</trt:ProfileToken>
    </trt:GetSnapshotUri>`, profileToken)

	resp, err := s.call(ctx, mediaEndpoint, "http://www.onvif.org/ver10/media/wsdl/GetSnapshotUri", user, pass, body)
	if err != nil {
		return "", err
	}

	rawURI := extractTag(resp, "Uri")
	if rawURI == "" {
		return "", fmt.Errorf("no snapshot uri returned in SOAP response")
	}
	return rawURI, nil
}

func (s *soapClient) fetchSnapshotBytes(ctx context.Context, snapshotURL, user, pass string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", snapshotURL, nil)
	if err != nil {
		return nil, err
	}
	if user != "" && pass != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// If 401 and Digest Auth is requested
	if resp.StatusCode == http.StatusUnauthorized && user != "" && pass != "" {
		authHeader := resp.Header.Get("WWW-Authenticate")
		if strings.HasPrefix(strings.ToLower(authHeader), "digest") {
			digestAuth := buildDigestAuthHeader(authHeader, "GET", snapshotURL, user, pass)
			if digestAuth != "" {
				req2, err2 := http.NewRequestWithContext(ctx, "GET", snapshotURL, nil)
				if err2 == nil {
					req2.Header.Set("Authorization", digestAuth)
					resp2, err3 := s.client.Do(req2)
					if err3 == nil {
						defer resp2.Body.Close()
						if resp2.StatusCode == http.StatusOK {
							return io.ReadAll(resp2.Body)
						}
					}
				}
			}
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot request returned HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func buildDigestAuthHeader(authHeader, method, rawURL, user, pass string) string {
	realm := extractDigestParam(authHeader, "realm")
	nonce := extractDigestParam(authHeader, "nonce")
	qop := extractDigestParam(authHeader, "qop")
	opaque := extractDigestParam(authHeader, "opaque")

	u, err := url.Parse(rawURL)
	uri := "/"
	if err == nil {
		uri = u.RequestURI()
	}

	ha1 := fmt.Sprintf("%x", sha1OrMD5(fmt.Sprintf("%s:%s:%s", user, realm, pass)))
	ha2 := fmt.Sprintf("%x", sha1OrMD5(fmt.Sprintf("%s:%s", method, uri)))

	nc := "00000001"
	cnonce := fmt.Sprintf("%x", time.Now().UnixNano())

	var response string
	if strings.Contains(qop, "auth") {
		response = fmt.Sprintf("%x", sha1OrMD5(fmt.Sprintf("%s:%s:%s:%s:auth:%s", ha1, nonce, nc, cnonce, ha2)))
		header := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", qop=auth, nc=%s, cnonce="%s"`,
			user, realm, nonce, uri, response, nc, cnonce)
		if opaque != "" {
			header += fmt.Sprintf(`, opaque="%s"`, opaque)
		}
		return header
	}

	response = fmt.Sprintf("%x", sha1OrMD5(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2)))
	header := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		user, realm, nonce, uri, response)
	if opaque != "" {
		header += fmt.Sprintf(`, opaque="%s"`, opaque)
	}
	return header
}

func sha1OrMD5(data string) [16]byte {
	return md5Sum(data)
}

func md5Sum(data string) [16]byte {
	return md5.Sum([]byte(data))
}

func md5Wrapper(b []byte) [16]byte {
	return md5.Sum(b)
}

func extractDigestParam(header, key string) string {
	re := regexp.MustCompile(fmt.Sprintf(`%s="?([^",]+)"?`, key))
	m := re.FindStringSubmatch(header)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractTag(xmlStr, tagName string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?i)<[^>]*%s[^>]*>([^<]+)</`, tagName))
	m := re.FindStringSubmatch(xmlStr)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
