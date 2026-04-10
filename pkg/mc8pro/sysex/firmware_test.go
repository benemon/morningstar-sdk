package sysex_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// TestDecodeFirmwareFromCapturedFrame parses a real firmware frame
// captured from the live MC8 Pro and asserts the values match what
// we know about this specific device (firmware 3.13.6, serial
// 77 4D 26 15 — see CLAUDE.md).
func TestDecodeFirmwareFromCapturedFrame(t *testing.T) {
	path := filepath.Join("..", "testdata", "raw", "032_1103_len0037.sysex")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture not present (run cmd/mccapture to regenerate): %v", err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	info, err := sysex.DecodeFirmwareFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFirmwareFrame: %v", err)
	}

	if info.Model != 8 {
		t.Errorf("Model = %d, want 8", info.Model)
	}
	if info.Major != 3 || info.Minor != 13 || info.Patch != 6 {
		t.Errorf("Firmware = %d.%d.%d, want 3.13.6", info.Major, info.Minor, info.Patch)
	}
	wantSerial := [4]byte{0x77, 0x4D, 0x26, 0x15}
	if info.Serial != wantSerial {
		t.Errorf("Serial = % X, want % X", info.Serial, wantSerial)
	}
}
