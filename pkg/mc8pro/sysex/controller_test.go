package sysex_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// parseCapturedFrame loads and parses the captured frame for a
// command pair, skipping the test if no capture exists.
func parseCapturedFrame(t *testing.T, cmd1, cmd2 byte) sysex.Frame {
	t.Helper()
	framePath := findCapturedFrame(t, cmd1, cmd2)
	if framePath == "" {
		t.Skipf("no captured %02X %02X frame", cmd1, cmd2)
	}
	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

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

// TestWaveformEnginesDecodeCaptured decodes the captured 03 24 frame
// (waveform engines per the editor's dispatch): 8 engines, and the
// write encoding drops the per-entry engine number (dump entries are
// 4 bytes, write entries 3).
func TestWaveformEnginesDecodeCaptured(t *testing.T) {
	frame := parseCapturedFrame(t, 0x03, 0x24)
	engines, err := sysex.DecodeWaveformEngines(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(engines) != 8 {
		t.Fatalf("engine count: got %d, want 8", len(engines))
	}
	for i, e := range engines {
		if e.EngineNum != i {
			t.Errorf("engine %d: EngineNum got %d, want %d", i, e.EngineNum, i)
		}
	}

	reencoded := sysex.EncodeWaveformEngines(engines)
	want := make([]byte, 0, 25)
	want = append(want, 8)
	for _, e := range engines {
		want = append(want, byte(e.Min), byte(e.Max), byte(e.Type))
	}
	if !bytes.Equal(reencoded, want) {
		t.Errorf("write payload: got % X, want % X", reencoded, want)
	}
}

// TestScrollCountersRoundTrip decodes the captured 03 26 frame
// (scroll counters per the editor's dispatch). Dump and write share
// the layout, so re-encoding must reproduce the payload exactly.
func TestScrollCountersRoundTrip(t *testing.T) {
	frame := parseCapturedFrame(t, 0x03, 0x26)
	counters, err := sysex.DecodeScrollCounters(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(counters) != 16 {
		t.Fatalf("counter count: got %d, want 16", len(counters))
	}
	reencoded := sysex.EncodeScrollCounters(counters)
	if !bytes.Equal(reencoded, frame.Payload) {
		t.Errorf("re-encode: got % X, want % X", reencoded, frame.Payload)
	}
}

// TestResistorLadderRoundTrip decodes the captured 03 28 frame (aux
// switch calibration per the editor's dispatch). The device's dump is
// one byte short of 16 full entries — the decoder must zero-fill the
// final switch's missing f2 byte, and the re-encode must match the
// capture across the bytes the capture actually has.
func TestResistorLadderRoundTrip(t *testing.T) {
	frame := parseCapturedFrame(t, 0x03, 0x28)
	switches, err := sysex.DecodeResistorLadder(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(switches) != 16 {
		t.Fatalf("switch count: got %d, want 16", len(switches))
	}
	if last := switches[15]; last.F2 != 0 {
		t.Errorf("truncated final entry: F2 got %d, want 0", last.F2)
	}
	reencoded := sysex.EncodeResistorLadder(switches)
	if len(reencoded) != 1+16*4 {
		t.Fatalf("write payload length: got %d, want %d", len(reencoded), 1+16*4)
	}
	if !bytes.Equal(reencoded[:len(frame.Payload)], frame.Payload) {
		t.Errorf("re-encode: got % X, want % X", reencoded[:len(frame.Payload)], frame.Payload)
	}
}

// TestSequencerEnginesRoundTrip decodes the captured 03 25 frame
// (sequencer engines per the editor's dispatch): 8 engines × 18-byte
// entries, an exact fit for the 145-byte payload. Dump and write
// share the layout.
func TestSequencerEnginesRoundTrip(t *testing.T) {
	frame := parseCapturedFrame(t, 0x03, 0x25)
	engines, err := sysex.DecodeSequencerEngines(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(engines) != 8 {
		t.Fatalf("engine count: got %d, want 8", len(engines))
	}
	reencoded := sysex.EncodeSequencerEngines(engines)
	if !bytes.Equal(reencoded, frame.Payload) {
		t.Errorf("re-encode: got % X, want % X", reencoded, frame.Payload)
	}
}

// TestOmniportsRoundTrip decodes the captured 03 23 frame (omniports
// per the editor's dispatch): 4 ports × 11-byte entries. The write
// encoding drops the dump's leading count byte, so the re-encode must
// equal payload[1:] over the ports' data.
func TestOmniportsRoundTrip(t *testing.T) {
	frame := parseCapturedFrame(t, 0x03, 0x23)
	ports, err := sysex.DecodeOmniports(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 4 {
		t.Fatalf("port count: got %d, want 4", len(ports))
	}
	reencoded := sysex.EncodeOmniports(ports)
	if !bytes.Equal(reencoded, frame.Payload[1:1+len(reencoded)]) {
		t.Errorf("re-encode: got % X, want % X", reencoded, frame.Payload[1:1+len(reencoded)])
	}
}

// TestBankArrangementDecodeCaptured decodes the captured 03 22 frame
// (bank arrangement per the editor's dispatch): the full 128-slot
// order table starting at payload byte 10.
func TestBankArrangementDecodeCaptured(t *testing.T) {
	frame := parseCapturedFrame(t, 0x03, 0x22)
	ba, err := sysex.DecodeBankArrangement(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if ba.IsActive {
		t.Error("IsActive: got true, want false (arranger disabled on test device)")
	}
	// The test device's table is the identity order 0..126 with the
	// final slot at the 127 sentinel.
	for i := 0; i < 127; i++ {
		if ba.BankOrder[i] != i {
			t.Fatalf("BankOrder[%d]: got %d, want %d", i, ba.BankOrder[i], i)
		}
	}
	if ba.BankOrder[127] != 127 {
		t.Errorf("BankOrder[127]: got %d, want 127 (unused sentinel)", ba.BankOrder[127])
	}
	if ba.NumBanksUsed != 127 {
		t.Errorf("NumBanksUsed: got %d, want 127", ba.NumBanksUsed)
	}

	// The write direction re-derives the count field from the active
	// list length (editor.js:15654-15666), so byte [1] becomes
	// count-1 rather than the dump's 0; the order table is copied
	// verbatim.
	reencoded := sysex.EncodeBankArrangement(ba)
	if len(reencoded) != len(frame.Payload) {
		t.Fatalf("write payload length: got %d, want %d", len(reencoded), len(frame.Payload))
	}
	if reencoded[1] != 126 {
		t.Errorf("count field: got %d, want 126 (active count 127 - 1)", reencoded[1])
	}
	if !bytes.Equal(reencoded[10:], frame.Payload[10:]) {
		t.Error("bank order table not copied verbatim")
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

// TestMidiEventsRoundTrip decodes the captured 03 27 frame (MIDI
// event processor per the editor's dispatch): 16 × 14-byte 7F-framed
// rows, an exact fit for the 224-byte payload. Dump and write share
// the layout.
func TestMidiEventsRoundTrip(t *testing.T) {
	frame := parseCapturedFrame(t, 0x03, 0x27)
	events, err := sysex.DecodeMidiEvents(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	reencoded := sysex.EncodeMidiEvents(events)
	if !bytes.Equal(reencoded, frame.Payload) {
		t.Errorf("re-encode: got % X, want % X", reencoded, frame.Payload)
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

// jsonSection is the {type, data: [{type, data: T}]} wrapper the
// editor uses for every controller-settings sub-section in a JSON
// backup.
type jsonSection[T any] struct {
	Data []struct {
		Data T `json:"data"`
	} `json:"data"`
	IsActive       bool `json:"isActive"`       // bank_arrangement only
	NumBanksActive int  `json:"numBanksActive"` // bank_arrangement only
}

func sectionValues[T any](t *testing.T, raw json.RawMessage) []T {
	t.Helper()
	var s jsonSection[T]
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("section unmarshal: %v", err)
	}
	out := make([]T, len(s.Data))
	for i, e := range s.Data {
		out[i] = e.Data
	}
	return out
}

// TestControllerSectionsMatchJSONBackup is the load-bearing
// verification for the 03 22–03 28 frame remap: it decodes each
// captured controller-settings frame and requires the decoded VALUES
// to equal the corresponding section of the all-banks JSON backup
// exported by the editor from the same device state. This is a
// value-level cross-source check — decoder against capture against
// editor export — so a codec keyed to the wrong frame, or fitting the
// right frame with the wrong structure, fails here even when its own
// encode/decode round-trip is self-consistent.
func TestControllerSectionsMatchJSONBackup(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "all-banks.json"))
	if err != nil {
		t.Skipf("no all-banks fixture: %v", err)
	}
	var dump struct {
		Data struct {
			ControllerSettings struct {
				Data map[string]json.RawMessage `json:"data"`
			} `json:"controller_settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatal(err)
	}
	sections := dump.Data.ControllerSettings.Data

	t.Run("waveform_engines", func(t *testing.T) {
		frame := parseCapturedFrame(t, 0x03, 0x24)
		got, err := sysex.DecodeWaveformEngines(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		want := sectionValues[mc8pro.WaveformEngine](t, sections["waveform_engines"])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("decoded:\n%+v\njson:\n%+v", got, want)
		}
	})

	t.Run("scroll_counters", func(t *testing.T) {
		frame := parseCapturedFrame(t, 0x03, 0x26)
		got, err := sysex.DecodeScrollCounters(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		want := sectionValues[mc8pro.ScrollCounter](t, sections["scroll_counters"])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("decoded:\n%+v\njson:\n%+v", got, want)
		}
	})

	t.Run("omniports", func(t *testing.T) {
		frame := parseCapturedFrame(t, 0x03, 0x23)
		got, err := sysex.DecodeOmniports(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		want := sectionValues[mc8pro.OmniportInput](t, sections["omniports"])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("decoded:\n%+v\njson:\n%+v", got, want)
		}
	})

	t.Run("sequencer_engines", func(t *testing.T) {
		frame := parseCapturedFrame(t, 0x03, 0x25)
		got, err := sysex.DecodeSequencerEngines(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		want := sectionValues[mc8pro.SequencerEngine](t, sections["sequencer_engines"])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("decoded:\n%+v\njson:\n%+v", got, want)
		}
	})

	t.Run("resistor_ladder_aux", func(t *testing.T) {
		frame := parseCapturedFrame(t, 0x03, 0x28)
		got, err := sysex.DecodeResistorLadder(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		want := sectionValues[mc8pro.ResistorLadderSwitch](t, sections["resistor_ladder_aux"])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("decoded:\n%+v\njson:\n%+v", got, want)
		}
	})

	t.Run("midi_events", func(t *testing.T) {
		frame := parseCapturedFrame(t, 0x03, 0x27)
		got, err := sysex.DecodeMidiEvents(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		want := sectionValues[mc8pro.MidiEvent](t, sections["midi_events"])
		if !reflect.DeepEqual(got[:], want) {
			t.Errorf("decoded:\n%+v\njson:\n%+v", got, want)
		}
	})

	t.Run("bank_arrangement", func(t *testing.T) {
		frame := parseCapturedFrame(t, 0x03, 0x22)
		got, err := sysex.DecodeBankArrangement(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var s jsonSection[struct {
			BankNum int `json:"bankNum"`
		}]
		if err := json.Unmarshal(sections["bank_arrangement"], &s); err != nil {
			t.Fatal(err)
		}
		if got.IsActive != s.IsActive {
			t.Errorf("IsActive: decoded %v, json %v", got.IsActive, s.IsActive)
		}
		// The JSON's numBanksActive is NOT compared: the editor's
		// device-read path fills its active list without updating
		// that counter, so backups exported after a device read
		// carry 0 there regardless of list length. The entry list
		// itself is the ground truth.
		if got.NumBanksUsed != len(s.Data) {
			t.Errorf("NumBanksUsed: decoded %d, json has %d entries", got.NumBanksUsed, len(s.Data))
		}
		for i, e := range s.Data {
			if got.BankOrder[i] != e.Data.BankNum {
				t.Fatalf("BankOrder[%d]: decoded %d, json %d", i, got.BankOrder[i], e.Data.BankNum)
			}
		}
	})
}
