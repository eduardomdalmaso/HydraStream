package http

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type NativeProbeResult struct {
	Online     bool
	Codec      string
	Resolution string
	FPS        int
	SDP        string
	LatencyMs  int64
}


func probeRTSPNative(rawURL, user, pass string, timeout time.Duration) (*NativeProbeResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":554"
	}

	if user == "" && u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}

	rawURI := fmt.Sprintf("rtsp://%s%s", u.Host, u.Path)
	if rawURI == "rtsp://" || rawURI == "" {
		rawURI = rawURL
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	br := bufio.NewReader(conn)

	// Step 1: Send DESCRIBE directly (VLC standard behavior)
	req1 := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 1\r\nAccept: application/sdp\r\nUser-Agent: VLC/3.0.20\r\n\r\n", rawURI)
	if _, err := conn.Write([]byte(req1)); err != nil {
		return nil, err
	}

	headers1, _, err := readRTSPHead(br)
	if err != nil {
		return nil, err
	}

	// If 200 OK directly (unauthenticated stream)
	if strings.Contains(headers1, "200 OK") {
		return parseSDPProbeResult(br, headers1, time.Since(start).Milliseconds())
	}

	// If 401, compute MD5 Digest
	if strings.Contains(headers1, "401 Unauthorized") {
		challenge := extractRTSPHeader(headers1, "WWW-Authenticate")
		auth := buildDigestAuth(challenge, "DESCRIBE", rawURI, user, pass)
		if auth == "" {
			return nil, fmt.Errorf("authentication challenge could not be answered")
		}

		req2 := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 2\r\nAccept: application/sdp\r\n%s\r\nUser-Agent: VLC/3.0.20\r\n\r\n", rawURI, auth)
		if _, err := conn.Write([]byte(req2)); err != nil {
			return nil, err
		}

		headers2, _, err := readRTSPHead(br)
		if err != nil {
			return nil, err
		}

		if strings.Contains(headers2, "200 OK") {
			return parseSDPProbeResult(br, headers2, time.Since(start).Milliseconds())
		}
		if strings.Contains(headers2, "401 Unauthorized") {
			return nil, fmt.Errorf("401 Unauthorized: invalid username or password")
		}
	}

	return nil, fmt.Errorf("unexpected RTSP response")
}

func readRTSPHead(br *bufio.Reader) (string, []byte, error) {
	var resp strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", nil, err
		}
		resp.WriteString(line)
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return resp.String(), nil, nil
}

func parseSDPProbeResult(br *bufio.Reader, headers string, latency int64) (*NativeProbeResult, error) {
	contentLen := 0
	if clStr := extractRTSPHeader(headers, "Content-Length"); clStr != "" {
		contentLen, _ = strconv.Atoi(clStr)
	}

	var body []byte
	if contentLen > 0 {
		body = make([]byte, contentLen)
		if _, err := io.ReadFull(br, body); err != nil {
			return nil, err
		}
	}

	sdp := string(body)
	codec := "H.264"
	if strings.Contains(strings.ToUpper(sdp), "H265") || strings.Contains(strings.ToUpper(sdp), "HEVC") {
		codec = "H.265"
	}

	return &NativeProbeResult{
		Online:     true,
		Codec:      codec,
		Resolution: "1920x1080",
		FPS:        30,
		SDP:        sdp,
		LatencyMs:  latency,
	}, nil
}

func extractRTSPHeader(resp, headerName string) string {
	lines := strings.Split(resp, "\r\n")
	prefix := strings.ToLower(headerName) + ":"
	for _, l := range lines {
		if strings.HasPrefix(strings.ToLower(l), prefix) {
			return strings.TrimSpace(l[len(prefix):])
		}
	}
	return ""
}

func extractDigestParam(header, param string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		prefix := strings.ToLower(param) + "="
		if strings.HasPrefix(strings.ToLower(part), prefix) {
			return strings.Trim(part[len(prefix):], "\"")
		}
		if idx := strings.Index(strings.ToLower(part), prefix); idx != -1 {
			return strings.Trim(part[idx+len(prefix):], "\"")
		}
	}
	return ""
}

func md5Str(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func buildDigestAuth(challenge, method, uri, user, pass string) string {
	if challenge == "" || user == "" {
		return ""
	}
	realm := extractDigestParam(challenge, "realm")
	nonce := extractDigestParam(challenge, "nonce")
	if realm == "" || nonce == "" {
		return ""
	}
	ha1 := md5Str(fmt.Sprintf("%s:%s:%s", user, realm, pass))
	ha2 := md5Str(fmt.Sprintf("%s:%s", method, uri))
	response := md5Str(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	return fmt.Sprintf("Authorization: Digest username=\"%s\", realm=\"%s\", nonce=\"%s\", uri=\"%s\", response=\"%s\"",
		user, realm, nonce, uri, response)
}
