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
// inverses of each other, using a synthetic preset built from the
// test fixture. This test runs unconditionally — no captured wire
// bytes are required.
//
// It ONLY asserts equality of the fields that DecodePresetFrame
// currently decodes (preset number, message array, and name fields).
// Color/flag fields from the config row are not yet round-tripped and
// are excluded from the comparison.
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

			// Trim original to only the fields the current decoder
			// round-trips, so this test doesn't fail on fields we
			// haven't decoded yet.
			want := trimToDecoded(original)
			got := trimToDecoded(decoded)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("round-trip mismatch\n want: %+v\n  got: %+v", want, got)
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

	// Message comparison: populated slots must match byte-for-byte;
	// empty slots must be empty in both. Any difference is reported
	// per-slot with explicit field names.
	for i := range decoded.MsgArray {
		dm := decoded.MsgArray[i]
		em := expected.MsgArray[i]
		switch {
		case dm.Type == 0 && em.Type == 0:
			// Both empty. Field values in the other fields are
			// don't-care; skip comparison.
		case dm.Type == 0:
			t.Errorf("slot %d: expected populated (Type=%d), got empty", i, em.Type)
		case em.Type == 0:
			t.Errorf("slot %d: expected empty, got populated (Type=%d, Data=%v)", i, dm.Type, dm.Data)
		default:
			if !reflect.DeepEqual(dm, em) {
				t.Errorf("slot %d: populated message mismatch\n got: %+v\nwant: %+v", i, dm, em)
			}
		}
	}
}

// trimToDecoded zeros out every field that DecodePresetFrame does
// NOT currently populate, so round-trip tests don't fail on fields
// we haven't implemented yet. As decode coverage expands, this
// function should shrink.
func trimToDecoded(p mc8pro.Preset) mc8pro.Preset {
	return mc8pro.Preset{
		PresetNum:  p.PresetNum,
		ShortName:  p.ShortName,
		ToggleName: p.ToggleName,
		LongName:   p.LongName,
		ShiftName:  p.ShiftName,
		MsgArray:   p.MsgArray,
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

func presetTestName(idx int, p mc8pro.Preset) string {
	if p.ShortName != "" {
		return "preset_" + byteHex(byte(idx)) + "_" + p.ShortName
	}
	return "preset_" + byteHex(byte(idx))
}
