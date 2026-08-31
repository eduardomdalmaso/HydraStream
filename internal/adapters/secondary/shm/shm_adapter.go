package shm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	HydraMagic   uint32 = 0x48594452 // "HYDR"
	HydraVersion uint32 = 1
)

// Header represents the binary layout of the Rust ShmHeader.
type Header struct {
	Magic         uint32
	Version       uint32
	Width         uint32
	Height        uint32
	Format        uint32
	SlotCount     uint32
	SlotSize      uint32
	WriteSequence uint64
}

// Manager inspects and interacts with POSIX shared memory ring buffers.
type Manager struct {
	baseDir string
}

// NewManager creates a Manager targeting /dev/shm or temp dir.
func NewManager() *Manager {
	dir := "/dev/shm"
	if _, err := os.Stat(dir); err != nil {
		dir = os.TempDir()
	}
	return &Manager{baseDir: dir}
}

// InspectStreamSHM reads header information from an active shared memory buffer.
func (m *Manager) InspectStreamSHM(streamID string) (*Header, error) {
	filePath := filepath.Join(m.baseDir, fmt.Sprintf("hydra_%s", streamID))
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hdr Header
	if err := binary.Read(f, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("failed to read SHM header: %w", err)
	}

	if hdr.Magic != HydraMagic {
		return nil, errors.New("invalid HydraStream SHM magic identifier")
	}

	return &hdr, nil
}

// ReadLatestSequence returns the current write sequence from an active stream SHM.
func (m *Manager) ReadLatestSequence(streamID string) (uint64, error) {
	hdr, err := m.InspectStreamSHM(streamID)
	if err != nil {
		return 0, err
	}
	return hdr.WriteSequence, nil
}

// WriteMockHeader creates a temporary mock SHM file for testing in Go.
func (m *Manager) WriteMockHeader(streamID string, width, height uint32, slots uint32) (string, error) {
	filePath := filepath.Join(m.baseDir, fmt.Sprintf("hydra_%s", streamID))
	f, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hdr := Header{
		Magic:         HydraMagic,
		Version:       HydraVersion,
		Width:         width,
		Height:        height,
		Format:        1,
		SlotCount:     slots,
		SlotSize:      width * height * 3,
		WriteSequence: 42,
	}

	if err := binary.Write(f, binary.LittleEndian, hdr); err != nil {
		return "", err
	}

	// Pad file to mock slot size
	if _, err := f.Seek(int64(hdr.SlotSize), io.SeekStart); err == nil {
		f.Write([]byte{0})
	}

	return filePath, nil
}

// Cleanup removes a mock SHM file.
func (m *Manager) Cleanup(streamID string) {
	filePath := filepath.Join(m.baseDir, fmt.Sprintf("hydra_%s", streamID))
	_ = os.Remove(filePath)
}
