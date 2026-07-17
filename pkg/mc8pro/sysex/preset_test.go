package sysex_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// TestPresetEncodeDecodeRoundTrip verifies that encode and decode are
// inverses of each other, using presets from the JSON fixture. All
// decoded fields (header, messages, names, config row) are compared.
func TestPresetEncodeDecodeRoundTrip(t *testing.T) {
	dump := loadFixtureDump(t, "bank-guitar-live.json")
	if dump.Data.Bank == nil {
		t.Fatal("fixture is not a singleBank dump")
	}

	for i := range dump.Data.Bank.PresetArray {
		original := dump.Data.Bank.PresetArray[i]
		t.Run(presetTestName(i, original), func(t *testing.T) {
			payload := sysex.EncodePresetFrame(original)
			decoded, err := sysex.DecodePresetFrame(payload)
			if err != nil {
				t.Fatalf("DecodePresetFrame: %v", err)
			}

			if !reflect.DeepEqual(original, decoded) {
				t.Errorf("round-trip mismatch\n want: %+v\n  got: %+v", original, decoded)
			}
		})
	}
}

// TestPresetDecodeCapturedFrame compares the decoder's output on a
// captured wire frame against the JSON fixture. This is the
// load-bearing correctness test: if the populated messages in the
// decoded preset match what the editor would export, our decoder is
// provably correct for the fields it covers.
//
// We compare populated messages byte-for-byte and empty slots only by
// type. This is because the editor normalizes empty-slot field values
// on export (Channel=1, ToggleGroup=2) while the wire carries
// "leftover" values (Channel=2, ToggleGroup=1 or similar) that are
// semantically irrelevant when Type=0. Trying to match empty-slot
// defaults would be fighting a battle over don't-care values.
//
// Skipped when the capture fixture is absent (run `go run
// ./cmd/mccapture` with the pedal connected to generate it).
func TestPresetDecodeCapturedFrame(t *testing.T) {
	framePath := findCapturedFrame(t, 0x06, 0x01)
	if framePath == "" {
		t.Skipf("no captured 06 01 frame in testdata/raw/; run `go run ./cmd/mccapture` to generate")
	}

	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatalf("read %s: %v", framePath, err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", framePath, err)
	}
	decoded, err := sysex.DecodePresetFrame(frame.Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	dump := loadFixtureDump(t, "bank-guitar-live.json")
	if dump.Data.Bank == nil {
		t.Fatal("fixture is not a singleBank dump")
	}
	if decoded.PresetNum < 0 || decoded.PresetNum >= len(dump.Data.Bank.PresetArray) {
		t.Fatalf("decoded preset index %d out of range", decoded.PresetNum)
	}
	expected := dump.Data.Bank.PresetArray[decoded.PresetNum]

	// Name fields should match exactly.
	if decoded.ShortName != expected.ShortName {
		t.Errorf("ShortName: got %q, want %q", decoded.ShortName, expected.ShortName)
	}
	if decoded.LongName != expected.LongName {
		t.Errorf("LongName: got %q, want %q", decoded.LongName, expected.LongName)
	}
	if decoded.ToggleName != expected.ToggleName {
		t.Errorf("ToggleName: got %q, want %q", decoded.ToggleName, expected.ToggleName)
	}
	if decoded.ShiftName != expected.ShiftName {
		t.Errorf("ShiftName: got %q, want %q", decoded.ShiftName, expected.ShiftName)
	}

	// Config row fields should match.
	if decoded.ToToggle != expected.ToToggle {
		t.Errorf("ToToggle: got %v, want %v", decoded.ToToggle, expected.ToToggle)
	}
	if decoded.ToBlink != expected.ToBlink {
		t.Errorf("ToBlink: got %v, want %v", decoded.ToBlink, expected.ToBlink)
	}
	if decoded.ToMsgScroll != expected.ToMsgScroll {
		t.Errorf("ToMsgScroll: got %v, want %v", decoded.ToMsgScroll, expected.ToMsgScroll)
	}
	if decoded.ToggleGroup != expected.ToggleGroup {
		t.Errorf("ToggleGroup: got %d, want %d", decoded.ToggleGroup, expected.ToggleGroup)
	}
	if decoded.LedColor != expected.LedColor {
		t.Errorf("LedColor: got %d, want %d", decoded.LedColor, expected.LedColor)
	}
	if decoded.LedToggleColor != expected.LedToggleColor {
		t.Errorf("LedToggleColor: got %d, want %d", decoded.LedToggleColor, expected.LedToggleColor)
	}
	if decoded.LedShiftColor != expected.LedShiftColor {
		t.Errorf("LedShiftColor: got %d, want %d", decoded.LedShiftColor, expected.LedShiftColor)
	}
	if decoded.BackgroundColor != expected.BackgroundColor {
		t.Errorf("BackgroundColor: got %d, want %d", decoded.BackgroundColor, expected.BackgroundColor)
	}
	if decoded.NameColor != expected.NameColor {
		t.Errorf("NameColor: got %d, want %d", decoded.NameColor, expected.NameColor)
	}
	if decoded.NameToggleColor != expected.NameToggleColor {
		t.Errorf("NameToggleColor: got %d, want %d", decoded.NameToggleColor, expected.NameToggleColor)
	}
	if decoded.NameShiftColor != expected.NameShiftColor {
		t.Errorf("NameShiftColor: got %d, want %d", decoded.NameShiftColor, expected.NameShiftColor)
	}
	if decoded.ToggleBackgroundColor != expected.ToggleBackgroundColor {
		t.Errorf("ToggleBackgroundColor: got %d, want %d", decoded.ToggleBackgroundColor, expected.ToggleBackgroundColor)
	}
	if decoded.ShiftBackgroundColor != expected.ShiftBackgroundColor {
		t.Errorf("ShiftBackgroundColor: got %d, want %d", decoded.ShiftBackgroundColor, expected.ShiftBackgroundColor)
	}

	// Message comparison: compare populated message slots against the
	// JSON fixture. We only compare M, Type, Action, and Data — the
	// fields that define the message's behavior. Channel and
	// ToggleGroup are excluded because the editor's JSON export
	// normalizes these differently from the wire for some message
	// types (e.g. Type=15 internal messages: wire has c=2/tg=1,
	// editor exports c=1/tg=2). Empty slots are skipped entirely —
	// their wire defaults differ from JSON. Wire byte-level fidelity
	// for ALL slots is covered by TestPresetWireRoundTrip.
	for i := range decoded.MsgArray {
		dm := decoded.MsgArray[i]
		em := expected.MsgArray[i]
		if dm.Type == 0 && em.Type == 0 {
			continue // both empty
		}
		if dm.Type != em.Type {
			t.Errorf("slot %d: Type got %d, want %d", i, dm.Type, em.Type)
			continue
		}
		if dm.M != em.M {
			t.Errorf("slot %d: M got %d, want %d", i, dm.M, em.M)
		}
		if dm.Action != em.Action {
			t.Errorf("slot %d: Action got %d, want %d", i, dm.Action, em.Action)
		}
		if dm.Data != em.Data {
			t.Errorf("slot %d: Data got %v, want %v", i, dm.Data, em.Data)
		}
	}
}

// loadFixtureDump loads a JSON dump fixture from testdata/ (resolved
// relative to the sysex test file, which is one directory below the
// mc8 package).
func loadFixtureDump(t *testing.T, filename string) mc8pro.Dump {
	t.Helper()
	path := filepath.Join("..", "testdata", filename)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var d mc8pro.Dump
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return d
}

// findCapturedFrame looks for the first captured .sysex file with
// the given CMD1/CMD2 in testdata/raw/. Returns "" (and skips via
// the caller) if none exists. The filename format produced by
// mccapture is NNN_CMD1CMD2_lenNNNN.sysex.
func findCapturedFrame(t *testing.T, cmd1, cmd2 byte) string {
	t.Helper()
	rawDir := filepath.Join("..", "testdata", "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		return ""
	}
	suffix := byteHex(cmd1) + byteHex(cmd2)
	for _, e := range entries {
		name := e.Name()
		if len(name) < 7 {
			continue
		}
		// format: NNN_CMD1CMD2_lenNNNN.sysex
		// positions 4..8 hold the 4-char command hex
		if name[4:8] == suffix {
			return filepath.Join(rawDir, name)
		}
	}
	return ""
}

func byteHex(b byte) string {
	const hex = "0123456789ABCDEF"
	return string([]byte{hex[b>>4], hex[b&0xF]})
}

// TestPresetWireRoundTrip is the load-bearing fidelity test. It takes
// a real captured preset frame (wire bytes from the device), decodes
// it into a Preset, re-encodes it, and compares the resulting bytes
// against the original. Any difference means the SDK would corrupt
// the device state on a read-then-write cycle.
//
// This catches bugs that struct-level round-trip tests miss — e.g.
// "don't-care" fields in empty message slots that the device actually
// preserves, or config tail bytes that get zeroed.
func TestPresetWireRoundTrip(t *testing.T) {
	framePath := findCapturedFrame(t, 0x06, 0x01)
	if framePath == "" {
		t.Skipf("no captured 06 01 frame in testdata/raw/")
	}

	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatalf("read %s: %v", framePath, err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", framePath, err)
	}

	// Decode → encode → compare payload bytes.
	decoded, err := sysex.DecodePresetFrame(frame.Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	reencoded := sysex.EncodePresetFrame(decoded)

	if len(reencoded) != len(frame.Payload) {
		t.Fatalf("payload length: got %d, want %d", len(reencoded), len(frame.Payload))
	}

	// Compare byte-by-byte. The only accepted difference is the
	// config row tail (bytes 13–31 of the 32-byte config row): the
	// device sends 0x01 there but both the editor and SDK write
	// 0x00 (reserved padding). We identify the config row's position
	// dynamically by finding row tag 5.
	configTailStart, configTailEnd := findConfigTailRange(frame.Payload)

	diffs := 0
	for i := range frame.Payload {
		if reencoded[i] != frame.Payload[i] {
			if i >= configTailStart && i < configTailEnd {
				// Expected: device sends 0x01, we write 0x00.
				continue
			}
			if diffs < 20 {
				t.Errorf("byte[%d]: got 0x%02X, want 0x%02X", i, reencoded[i], frame.Payload[i])
			}
			diffs++
		}
	}
	if diffs > 20 {
		t.Errorf("... and %d more byte differences", diffs-20)
	}
	if diffs > 0 {
		t.Errorf("total unexpected byte differences: %d out of %d", diffs, len(frame.Payload))
	}
}

// findConfigTailRange returns the byte range [start, end) within a
// preset payload that corresponds to the reserved config tail bytes
// (bytes 13–31 of the 32-byte config row, tag 5).
func findConfigTailRange(payload []byte) (int, int) {
	i := 0
	for i < len(payload) {
		if payload[i] != 0x7F {
			break
		}
		tag := payload[i+1]
		length := int(payload[i+2])
		dataStart := i + 3
		if tag == 0x05 && length == 32 {
			// Config row: bytes 0-12 are decoded fields, 13-31 are reserved.
			return dataStart + 13, dataStart + 32
		}
		i = dataStart + length
	}
	return -1, -1 // not found; no exclusions
}

func presetTestName(idx int, p mc8pro.Preset) string {
	if p.ShortName != "" {
		return "preset_" + byteHex(byte(idx)) + "_" + p.ShortName
	}
	return "preset_" + byteHex(byte(idx))
}
