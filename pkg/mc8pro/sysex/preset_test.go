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

// Message-row bytes [5]–[7] are direction-asymmetric on the wire:
// dumps carry (action, toggleGroup, channel), writes must carry
// (channel, action, toggleGroup). Encode and decode are therefore
// deliberately NOT inverses, and there is no encode→decode round-trip
// test: such a test can only prove self-consistency, and a symmetric
// codec with the wrong field order passes it while corrupting every
// real write (this was the write-fidelity blocker). Each direction is
// instead pinned independently below.

// TestMessageRowDumpOrder pins the DECODE direction: a message row
// with three distinct values at bytes [5]–[7] must decode as
// (action, toggleGroup, channel).
func TestMessageRowDumpOrder(t *testing.T) {
	var payload []byte
	payload = sysex.BuildRow(payload, 0x00, []byte{3, 7, 0, 0}) // header: bank 3, preset 7
	row := make([]byte, 23)
	row[0] = 4    // m: slot 4
	row[1] = 2    // t: CC
	row[2] = 39   // data[0]: CC#
	row[3] = 64   // data[1]: value
	row[5] = 0x0A // dump order: action
	row[6] = 0x0B // dump order: toggleGroup
	row[7] = 0x0C // dump order: channel
	payload = sysex.BuildRow(payload, 0x01, row)

	p, err := sysex.DecodePresetFrame(payload)
	if err != nil {
		t.Fatalf("DecodePresetFrame: %v", err)
	}
	m := p.MsgArray[4]
	if m.Action != 0x0A || m.Toggle != 0x0B || m.Channel != 0x0C {
		t.Errorf("dump-order decode: got a=%d tg=%d c=%d, want a=10 tg=11 c=12",
			m.Action, m.Toggle, m.Channel)
	}
}

// TestMessageRowWriteOrder pins the ENCODE direction: a message with
// distinct field values must encode with (channel, action,
// toggleGroup) at bytes [5]–[7], matching the editor's getSysexArray
// (editor.js:14836-14858).
func TestMessageRowWriteOrder(t *testing.T) {
	var p mc8pro.Preset
	p.MsgArray[0] = mc8pro.Message{
		M: 0, Type: 2, Action: 0x0A, Toggle: 0x0B, Channel: 0x0C,
	}
	payload := sysex.EncodePresetFrame(p)

	// First message row follows the 7-byte header row: 7F 01 17 <23 bytes>.
	rowData := payload[7+3 : 7+3+23]
	if rowData[5] != 0x0C || rowData[6] != 0x0A || rowData[7] != 0x0B {
		t.Errorf("write-order encode: got [5]=%d [6]=%d [7]=%d, want [5]=c(12) [6]=a(10) [7]=tg(11)",
			rowData[5], rowData[6], rowData[7])
	}
}

// TestPresetDecodeCapturedFrame compares the decoder's output on a
// captured wire frame against the JSON fixture. This is the
// load-bearing correctness test: the capture and the fixture were
// taken from the SAME device state (the pre-corruption Phase 1
// session), so every decoded field — including Channel and
// Toggle (position) on empty slots — must match the editor's export
// exactly. Do not weaken this comparison: the original write-fidelity
// bug survived precisely because Channel/Toggle mismatches were
// excluded as "editor normalization" instead of investigated. The
// fixture pair contains an empty slot with three distinct a/tg/c
// values, which is what makes the field order provable.
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

	// Message comparison: every field on every slot, empty slots
	// included. The fixture and capture are from the same device
	// state, so any mismatch is a decoder bug — see the test comment.
	// Info (mi) is excluded: it is client-side editor metadata and
	// never appears on the wire.
	for i := range decoded.MsgArray {
		dm := decoded.MsgArray[i]
		em := expected.MsgArray[i]
		dm.Info = ""
		em.Info = ""
		if !reflect.DeepEqual(dm, em) {
			t.Errorf("slot %d:\n  got:  %+v\n  want: %+v", i, dm, em)
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

// TestPresetDumpToWriteTransform is the load-bearing fidelity test.
// It takes a real captured dump frame, decodes it, re-encodes it for
// the write direction, and requires the result to be EXACTLY the
// original bytes with the two documented, deliberate transformations
// applied:
//
//  1. every message row's bytes [5]–[7] permuted from dump order
//     (a, tg, c) to write order (c, a, tg), and
//  2. the config row tail (bytes 13–31 of row tag 5) zeroed — the
//     device dumps 0x01 there, the editor writes 0x00.
//
// Any other byte difference means a read-then-write cycle would
// corrupt device state.
func TestPresetDumpToWriteTransform(t *testing.T) {
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

	decoded, err := sysex.DecodePresetFrame(frame.Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	reencoded := sysex.EncodePresetFrame(decoded)

	expected := dumpToWritePayload(t, frame.Payload)
	if len(reencoded) != len(expected) {
		t.Fatalf("payload length: got %d, want %d", len(reencoded), len(expected))
	}

	diffs := 0
	for i := range expected {
		if reencoded[i] != expected[i] {
			if diffs < 20 {
				t.Errorf("byte[%d]: got 0x%02X, want 0x%02X", i, reencoded[i], expected[i])
			}
			diffs++
		}
	}
	if diffs > 0 {
		t.Errorf("total unexpected byte differences: %d out of %d", diffs, len(expected))
	}
}

// dumpToWritePayload applies the documented dump→write transformations
// to a captured dump payload: message-row [5..7] permutation
// (a,tg,c)→(c,a,tg) and config-tail zeroing. It walks the 7F-framed
// rows directly, independent of the codec under test.
func dumpToWritePayload(t *testing.T, dump []byte) []byte {
	t.Helper()
	out := append([]byte(nil), dump...)
	i := 0
	for i+2 < len(out) {
		if out[i] != 0x7F {
			t.Fatalf("payload offset %d: expected row marker 0x7F, got 0x%02X", i, out[i])
		}
		tag, length := out[i+1], int(out[i+2])
		data := i + 3
		switch {
		case tag == 0x01 && length == 23:
			a, tg, c := out[data+5], out[data+6], out[data+7]
			out[data+5], out[data+6], out[data+7] = c, a, tg
		case tag == 0x05 && length == 32:
			for j := 13; j < 32; j++ {
				out[data+j] = 0
			}
		}
		i = data + length
	}
	return out
}
