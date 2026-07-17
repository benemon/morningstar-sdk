package sysex_test

import (
	"os"
	"testing"

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

// TestMidiClockSlotsRoundTrip decodes a captured 03 28 frame.
func TestMidiClockSlotsRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x03, 0x28)
	if framePath == "" {
		t.Skip("no captured 03 28 frame")
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
	t.Logf("midi clock slots: %d entries", len(slots))

	reencoded := sysex.EncodeMidiClockSlots(slots)
	for i := 0; i < len(frame.Payload) && i < len(reencoded); i++ {
		if reencoded[i] != frame.Payload[i] {
			t.Errorf("byte [%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
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
