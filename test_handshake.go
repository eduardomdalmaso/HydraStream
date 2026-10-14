package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

func extractHeader(resp, headerName string) string {
	lines := strings.Split(resp, "\r\n")
	prefix := strings.ToLower(headerName) + ":"
	for _, l := range lines {
		if strings.HasPrefix(strings.ToLower(l), prefix) {
			val := strings.TrimSpace(strings.TrimPrefix(l, l[:len(prefix)]))
			if idx := strings.Index(val, ";"); idx != -1 {
				val = val[:idx]
			}
			return val
		}
	}
	return ""
}

func extractParam(header, param string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), strings.ToLower(param)+"=") {
			val := strings.TrimPrefix(part, part[:len(param)+1])
			return strings.Trim(val, "\"")
		}
	}
	return ""
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func buildAuthHeader(challenge, method, uri, user, pass string) string {
	if challenge == "" || user == "" {
		return ""
	}
	realm := extractParam(challenge, "realm")
	nonce := extractParam(challenge, "nonce")
	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", user, realm, pass))
	ha2 := md5Hex(fmt.Sprintf("%s:%s", method, uri))
	response := md5Hex(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	return fmt.Sprintf("Authorization: Digest username=\"%s\", realm=\"%s\", nonce=\"%s\", uri=\"%s\", response=\"%s\"",
		user, realm, nonce, uri, response)
}

func main() {
	rawURL := "rtsp://komtek:Kt@128.201.243.154:7070/Streaming/channels/401/"
	u, _ := url.Parse(rawURL)
	rawURI := fmt.Sprintf("rtsp://%s%s", u.Host, u.Path)

	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		fmt.Printf("Dial err: %v\n", err)
		return
	}
	defer conn.Close()

	br := bufio.NewReader(conn)
	fmt.Fprintf(conn, "OPTIONS %s RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: HydraStream/1.0\r\n\r\n")

	var optHeaders string
	for {
		line, _ := br.ReadString('\n')
		optHeaders += line
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	challenge := extractHeader(optHeaders, "WWW-Authenticate")
	auth := buildAuthHeader(challenge, "DESCRIBE", rawURI, "komtek", "Kt")

	fmt.Fprintf(conn, "DESCRIBE %s RTSP/1.0\r\nCSeq: 2\r\nAccept: application/sdp\r\n%s\r\nUser-Agent: HydraStream/1.0\r\n\r\n", rawURI, auth)

	var descHeaders string
	for {
		line, _ := br.ReadString('\n')
		descHeaders += line
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	fmt.Printf("DESCRIBE Response Status:\n%s\n", strings.Split(descHeaders, "\r\n")[0])
}
