package sysex_test

import (
	"os"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// TestControllerConfigRoundTrip decodes a captured 03 21 frame (global
// controller settings, verified by JSON correlation), re-encodes, and
// verifies byte-level fidelity.
func TestControllerConfigRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x21)
	if framePath == "" {
		t.Skip("no captured 03 21 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sysex.DecodeControllerConfig(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("controller config: midiChannel=%d, brightness=%d, switchSensitivity=%d",
		cfg.MidiChannel, cfg.BrightnessValue, cfg.SwitchSensitivity)

	reencoded := sysex.EncodeControllerConfig(cfg)
	compareLen := 32
	if len(frame.Payload) < compareLen {
		compareLen = len(frame.Payload)
	}
	for i := 0; i < compareLen; i++ {
		if reencoded[i] != frame.Payload[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
		}
	}
}

// TestWaveformEnginesRoundTrip decodes a captured 03 26 frame.
func TestWaveformEnginesRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x26)
	if framePath == "" {
		t.Skip("no captured 03 26 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	engines, err := sysex.DecodeWaveformEngines(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("waveform engines: %d entries", len(engines))

	reencoded := sysex.EncodeWaveformEngines(engines)
	encodedLen := 1 + len(engines)*3
	for i := 0; i < encodedLen; i++ {
		if reencoded[i] != frame.Payload[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
		}
	}
}

// TestResistorLadderRoundTrip decodes a captured 03 25 frame.
func TestResistorLadderRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x25)
	if framePath == "" {
		t.Skip("no captured 03 25 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	switches, err := sysex.DecodeResistorLadder(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("resistor ladder: %d switches", len(switches))

	reencoded := sysex.EncodeResistorLadder(switches)
	dataLen := 1 + len(switches)*4
	for i := 0; i < dataLen; i++ {
		if reencoded[i] != frame.Payload[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
		}
	}
}

// TestSequencerEnginesRoundTrip decodes a captured 03 23 frame.
func TestSequencerEnginesRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x23)
	if framePath == "" {
		t.Skip("no captured 03 23 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	engines, err := sysex.DecodeSequencerEngines(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sequencer engines: %d entries", len(engines))

	reencoded := sysex.EncodeSequencerEngines(engines)
	dataLen := 1 + len(engines)*11
	for i := 0; i < dataLen; i++ {
		if reencoded[i] != frame.Payload[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
		}
	}
}

// TestOmniportsRoundTrip decodes a captured 03 24 frame.
func TestOmniportsRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x24)
	if framePath == "" {
		t.Skip("no captured 03 24 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	ports, err := sysex.DecodeOmniports(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("omniports: %d entries", len(ports))

	reencoded := sysex.EncodeOmniports(ports)
	dataLen := 1 + len(ports)*4
	for i := 0; i < dataLen; i++ {
		if reencoded[i] != frame.Payload[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
		}
	}
}

// TestMidiChannelsRoundTrip decodes a captured 03 20 frame and
// verifies that channel 2 (tag 0x01) contains "Quad Cortex Mini".
func TestMidiChannelsRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x20)
	if framePath == "" {
		t.Skip("no captured 03 20 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := sysex.DecodeMidiChannels(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}

	// Channel 2 (index 1) should be "Quad Cortex Mini".
	if channels[1].Name != "Quad Cortex Mini" {
		t.Errorf("channel 1 name: got %q, want %q", channels[1].Name, "Quad Cortex Mini")
	} else {
		t.Logf("channel 1 (MIDI ch 2): %q ✓", channels[1].Name)
	}

	// Encode→decode round-trip.
	reencoded := sysex.EncodeMidiChannels(channels)
	decoded2, err := sysex.DecodeMidiChannels(reencoded)
	if err != nil {
		t.Fatal(err)
	}
	for i := range channels {
		if channels[i].Name != decoded2[i].Name {
			t.Errorf("channel %d name round-trip: got %q, want %q", i, decoded2[i].Name, channels[i].Name)
		}
	}
}

// TestMidiClockSlotsDecodeCaptured decodes the captured 03 29 frame
// (MIDI clock slots per the editor's dispatch). The test device has
// 16 unconfigured slots, so the capture is [reserved, 0x10, 32 zero
// BPM bytes].
func TestMidiClockSlotsDecodeCaptured(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x29)
	if framePath == "" {
		t.Skip("no captured 03 29 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	slots, err := sysex.DecodeMidiClockSlots(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 16 {
		t.Errorf("slot count: got %d, want 16", len(slots))
	}
	for i, s := range slots {
		if s.BPM != 0 {
			t.Errorf("slot %d: BPM got %d, want 0 (unconfigured)", i, s.BPM)
		}
	}
}

// TestMidiClockSlotsEncodeWriteOrder pins the WRITE payload shape
// against the editor's getArray() output: [count, (lsb, msb) × count]
// with no leading reserved byte (unlike the dump).
func TestMidiClockSlotsEncodeWriteOrder(t *testing.T) {
	// BPM 9999 must clamp to 500 (editor.js:13967) rather than
	// silently wrapping its two 7-bit bytes.
	slots := []mc8pro.MidiClockSlot{{BPM: 120}, {BPM: 9999}}
	got := sysex.EncodeMidiClockSlots(slots)
	want := []byte{2, 120, 0, 500 & 0x7F, 500 >> 7}
	if len(got) != len(want) {
		t.Fatalf("payload length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, got[i], want[i])
		}
	}
}

// TestMidiEventsRoundTrip decodes a captured 03 22 frame.
func TestMidiEventsRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x22)
	if framePath == "" {
		t.Skip("no captured 03 22 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := sysex.DecodeMidiEvents(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the remap table is an identity mapping on this device.
	for i := 0; i < 128; i++ {
		if ep.RemapTable[i] != byte(i) {
			t.Errorf("remap[%d] = %d, want %d", i, ep.RemapTable[i], i)
			break
		}
	}
	t.Log("remap table: identity mapping confirmed")

	reencoded := sysex.EncodeMidiEvents(ep)
	for i := 0; i < len(frame.Payload) && i < len(reencoded); i++ {
		if reencoded[i] != frame.Payload[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
		}
	}
}

// TestDecodeUUIDCaptured decodes the captured 11 00 frame and checks
// the result against the UUID observed live on the test device
// (State summary during the 2026-07-17 integration run).
func TestDecodeUUIDCaptured(t *testing.T) {
	framePath := findCapturedFrame(t, 0x11, 0x00)
	if framePath == "" {
		t.Skip("no captured 11 00 frame")
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	uuid, err := sysex.DecodeUUID(frame.Payload)
	if err != nil {
		t.Fatalf("DecodeUUID: %v", err)
	}
	const want = "6d550ea6330ba9d70000000000000000"
	if uuid != want {
		t.Errorf("uuid: got %s, want %s", uuid, want)
	}
}

// TestDecodeUUIDBadLength verifies the length guard.
func TestDecodeUUIDBadLength(t *testing.T) {
	if _, err := sysex.DecodeUUID(make([]byte, 16)); err == nil {
		t.Error("expected error for 16-byte payload, got nil")
	}
}
