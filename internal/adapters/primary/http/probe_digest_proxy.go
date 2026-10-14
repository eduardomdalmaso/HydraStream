package http

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
)

// startRTSPProxyServer launches a lightweight loopback RTSP proxy that authenticates
// with the camera using MD5 Digest on behalf of FFmpeg, bypassing Hikvision's OPTIONS 401 bug.
func startRTSPProxyServer(ctx context.Context, rawURL, user, pass string) (string, func(), error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, err
	}

	camHost := u.Host
	if !strings.Contains(camHost, ":") {
		camHost += ":554"
	}
	if user == "" && u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	camPath := u.Path
	if camPath == "" {
		camPath = "/Streaming/channels/401/"
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("proxy listen: %w", err)
	}

	localPort := listener.Addr().(*net.TCPAddr).Port
	proxyURL := fmt.Sprintf("rtsp://127.0.0.1:%d/live", localPort)

	cleanup := func() {
		_ = listener.Close()
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		for {
			ffmpegConn, err := listener.Accept()
			if err != nil {
				return
			}
			camConn, err := net.DialTimeout("tcp", camHost, 5*time.Second)
			if err != nil {
				_ = ffmpegConn.Close()
				continue
			}

			go func(ffC, camC net.Conn) {
				defer ffC.Close()
				defer camC.Close()
				go func() {
					<-ctx.Done()
					_ = ffC.Close()
					_ = camC.Close()
				}()
				_ = runProxyBridge(ffC, camC, camHost, camPath, user, pass)
			}(ffmpegConn, camConn)
		}
	}()

	return proxyURL, cleanup, nil
}

// captureSnapshotWithNativeDigestProxy uses the loopback RTSP proxy to grab a one-shot JPEG snapshot.
func captureSnapshotWithNativeDigestProxy(rawURL, user, pass, tmpFile string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	proxyURL, cleanup, err := startRTSPProxyServer(ctx, rawURL, user, pass)
	if err != nil {
		return err
	}
	defer cleanup()

	ffmpegBin := proc.GetFFmpegPath()
	snapCmd := exec.CommandContext(ctx, ffmpegBin,
		"-rtsp_transport", "tcp",
		"-allowed_media_types", "video",
		"-timeout", "6000000",
		"-i", proxyURL,
		"-update", "1",
		"-frames:v", "1",
		"-q:v", "2",
		"-y", tmpFile,
	)
	proc.SetHideWindow(snapCmd)

	_ = snapCmd.Run()
	if fi, errStat := os.Stat(tmpFile); errStat == nil && fi.Size() > 1000 {
		return nil
	}
	return fmt.Errorf("snapshot capture failed")
}

func runProxyBridge(ffmpegConn, camConn net.Conn, camHost, camPath, user, pass string) error {
	camReader := bufio.NewReader(camConn)
	ffReader := bufio.NewReader(ffmpegConn)

	var realm, nonce, session string
	var pendingMethod, pendingFFReq string
	camCSeq := 1

	// Goroutine: Forward Camera to FFmpeg
	go func() {
		for {
			firstByte, err := camReader.ReadByte()
			if err != nil {
				return
			}
			if firstByte == 0x24 { // '$' Interleaved RTP/RTCP packet
				channel, _ := camReader.ReadByte()
				var length uint16
				_ = binaryRead(camReader, &length)
				payload := make([]byte, length)
				_, _ = io.ReadFull(camReader, payload)
				hdr := []byte{0x24, channel, byte(length >> 8), byte(length & 0xff)}
				_, _ = ffmpegConn.Write(append(hdr, payload...))
				continue
			}

			line, err := readRTSPHeaderBlock(firstByte, camReader)
			if err != nil {
				return
			}
			respStr := string(line)

			if strings.Contains(respStr, "401 Unauthorized") {
				if rMatch := extractDigestParam(respStr, "realm"); rMatch != "" {
					realm = rMatch
				}
				if nMatch := extractDigestParam(respStr, "nonce"); nMatch != "" {
					nonce = nMatch
				}

				if pendingMethod == "DESCRIBE" {
					uri := fmt.Sprintf("rtsp://%s%s", camHost, camPath)
					auth := buildDigestAuth(fmt.Sprintf("realm=\"%s\", nonce=\"%s\"", realm, nonce), "DESCRIBE", uri, user, pass)
					req := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: %d\r\n%s\r\nUser-Agent: VLC/3.0.20\r\nAccept: application/sdp\r\n\r\n", uri, camCSeq, auth)
					camCSeq++
					_, _ = camConn.Write([]byte(req))
					continue
				}
				if pendingMethod == "SETUP" {
					uri := fmt.Sprintf("rtsp://%s%strackID=video", camHost, camPath)
					auth := buildDigestAuth(fmt.Sprintf("realm=\"%s\", nonce=\"%s\"", realm, nonce), "SETUP", uri, user, pass)
					req := fmt.Sprintf("SETUP %s RTSP/1.0\r\nCSeq: %d\r\n%s\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\nUser-Agent: VLC/3.0.20\r\n\r\n", uri, camCSeq, auth)
					camCSeq++
					_, _ = camConn.Write([]byte(req))
					continue
				}
				if pendingMethod == "PLAY" {
					uri := fmt.Sprintf("rtsp://%s%s", camHost, camPath)
					auth := buildDigestAuth(fmt.Sprintf("realm=\"%s\", nonce=\"%s\"", realm, nonce), "PLAY", uri, user, pass)
					req := fmt.Sprintf("PLAY %s RTSP/1.0\r\nCSeq: %d\r\n%s\r\nSession: %s\r\nUser-Agent: VLC/3.0.20\r\n\r\n", uri, camCSeq, auth, session)
					camCSeq++
					_, _ = camConn.Write([]byte(req))
					continue
				}
			}

			if strings.Contains(respStr, "200 OK") {
				if sess := extractRTSPHeader(respStr, "Session"); sess != "" {
					session = strings.Split(sess, ";")[0]
				}
				ffCSeq := extractRTSPHeader(pendingFFReq, "CSeq")
				if ffCSeq != "" {
					cseqRe := regexp.MustCompile(`(?i)CSeq:\s*\d+`)
					respStr = cseqRe.ReplaceAllString(respStr, fmt.Sprintf("CSeq: %s", ffCSeq))
				}
				_, _ = ffmpegConn.Write([]byte(respStr))
				continue
			}

			_, _ = ffmpegConn.Write(line)
		}
	}()

	// Loop: Read from FFmpeg and handle
	for {
		firstByte, err := ffReader.ReadByte()
		if err != nil {
			return nil
		}
		if firstByte == 0x24 {
			ch, _ := ffReader.ReadByte()
			var length uint16
			_ = binaryRead(ffReader, &length)
			payload := make([]byte, length)
			_, _ = io.ReadFull(ffReader, payload)
			hdr := []byte{0x24, ch, byte(length >> 8), byte(length & 0xff)}
			_, _ = camConn.Write(append(hdr, payload...))
			continue
		}

		line, err := readRTSPHeaderBlock(firstByte, ffReader)
		if err != nil {
			return nil
		}
		reqStr := string(line)
		pendingFFReq = reqStr

		if strings.HasPrefix(reqStr, "OPTIONS") {
			cseq := extractRTSPHeader(reqStr, "CSeq")
			if cseq == "" {
				cseq = "1"
			}
			resp := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %s\r\nPublic: OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY\r\n\r\n", cseq)
			_, _ = ffmpegConn.Write([]byte(resp))
			continue
		}

		if strings.HasPrefix(reqStr, "DESCRIBE") {
			pendingMethod = "DESCRIBE"
			uri := fmt.Sprintf("rtsp://%s%s", camHost, camPath)
			req := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: %d\r\nUser-Agent: VLC/3.0.20\r\nAccept: application/sdp\r\n", uri, camCSeq)
			camCSeq++
			if realm != "" && nonce != "" {
				req += buildDigestAuth(fmt.Sprintf("realm=\"%s\", nonce=\"%s\"", realm, nonce), "DESCRIBE", uri, user, pass) + "\r\n"
			}
			req += "\r\n"
			_, _ = camConn.Write([]byte(req))
			continue
		}

		if strings.HasPrefix(reqStr, "SETUP") {
			pendingMethod = "SETUP"
			uri := fmt.Sprintf("rtsp://%s%strackID=video", camHost, camPath)
			req := fmt.Sprintf("SETUP %s RTSP/1.0\r\nCSeq: %d\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\nUser-Agent: VLC/3.0.20\r\n", uri, camCSeq)
			camCSeq++
			if session != "" {
				req += fmt.Sprintf("Session: %s\r\n", session)
			}
			if realm != "" && nonce != "" {
				req += buildDigestAuth(fmt.Sprintf("realm=\"%s\", nonce=\"%s\"", realm, nonce), "SETUP", uri, user, pass) + "\r\n"
			}
			req += "\r\n"
			_, _ = camConn.Write([]byte(req))
			continue
		}

		if strings.HasPrefix(reqStr, "PLAY") {
			pendingMethod = "PLAY"
			uri := fmt.Sprintf("rtsp://%s%s", camHost, camPath)
			req := fmt.Sprintf("PLAY %s RTSP/1.0\r\nCSeq: %d\r\nSession: %s\r\nUser-Agent: VLC/3.0.20\r\n", uri, camCSeq, session)
			camCSeq++
			if realm != "" && nonce != "" {
				req += buildDigestAuth(fmt.Sprintf("realm=\"%s\", nonce=\"%s\"", realm, nonce), "PLAY", uri, user, pass) + "\r\n"
			}
			req += "\r\n"
			_, _ = camConn.Write([]byte(req))
			continue
		}

		_, _ = camConn.Write(line)
	}
}

func binaryRead(r io.Reader, v *uint16) error {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	*v = (uint16(buf[0]) << 8) | uint16(buf[1])
	return nil
}

func readRTSPHeaderBlock(firstByte byte, r *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(firstByte)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return buf.Bytes(), err
		}
		buf.Write(line)
		if bytes.Equal(line, []byte("\r\n")) || bytes.Equal(line, []byte("\n")) {
			break
		}
	}
	contentLen := 0
	if clStr := extractRTSPHeader(buf.String(), "Content-Length"); clStr != "" {
		_, _ = fmt.Sscanf(clStr, "%d", &contentLen)
	}
	if contentLen > 0 {
		body := make([]byte, contentLen)
		if _, err := io.ReadFull(r, body); err == nil {
			buf.Write(body)
		}
	}
	return buf.Bytes(), nil
}
