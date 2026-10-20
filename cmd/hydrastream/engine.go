package main

import (
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"hydrastream/internal/adapters/secondary/proc"
)

func isPortInUse(port string) bool {
	ln, err := net.Listen("tcp", port)
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func buildOrFindRustEngine() string {
	targetBin := "hydra-engine"
	if runtime.GOOS == "windows" {
		targetBin += ".exe"
	}

	candidates := []string{
		filepath.Join("target", "release", targetBin),
		filepath.Join("target", "debug", targetBin),
		filepath.Join("bin", targetBin),
		targetBin,
	}

	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}

	log.Println("🔨 [HydraStream] Rust hydra-engine binary not found. Compiling via cargo...")
	buildCmd := exec.Command("cargo", "build", "--release", "--manifest-path", "crates/hydra-engine/Cargo.toml")
	proc.SetHideWindow(buildCmd)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		log.Printf("⚠️ [HydraStream] Failed to compile hydra-engine via cargo: %v\n", err)
		return ""
	}

	releasePath := filepath.Join("target", "release", targetBin)
	if fileExists(releasePath) {
		return releasePath
	}
	return ""
}

func startEmbeddedWspEngine() *exec.Cmd {
	binPath := buildOrFindRustEngine()
	if binPath == "" {
		log.Println("⚠️ [HydraStream] hydra-engine binary not available. Skipping embedded WSP server.")
		return nil
	}

	port := os.Getenv("WSP_PORT")
	if port == "" {
		if isPortInUse(":8085") {
			port = "8086"
		} else {
			port = "8085"
		}
	}

	rtmpPort := os.Getenv("RTMP_PORT")
	if rtmpPort == "" {
		if isPortInUse(":1935") {
			rtmpPort = "1936"
		} else {
			rtmpPort = "1935"
		}
	}

	checkAddr := ":" + port
	if isPortInUse(checkAddr) {
		log.Printf("ℹ️ [HydraStream] WSP Server port %s already active. Attaching to existing instance.\n", checkAddr)
		return nil
	}

	cmd := exec.Command(binPath, "server", "--port", port, "--rtmp-port", rtmpPort)
	proc.SetHideWindow(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("❌ [HydraStream] Failed to start embedded Rust WSP Engine: %v\n", err)
		return nil
	}

	log.Printf("🚀 [HydraStream] Embedded Rust WSP Engine started (PID %d) on ws://0.0.0.0:%s (RTMP push :%s)\n", cmd.Process.Pid, port, rtmpPort)
	return cmd
}
