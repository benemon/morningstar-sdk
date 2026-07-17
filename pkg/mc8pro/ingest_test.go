package mc8pro

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// silentLogger is the shared no-op logger for ingest tests. Tests
// don't care about the debug output; they care about the State
// mutations.
var silentLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// parseFixture reads a .sysex file from testdata/raw/ and returns the
// parsed frame. t.Skip if the file is absent so tests still run on
// machines without the hardware-captured fixtures.
func parseFixture(t *testing.T, filename string) sysex.Frame {
	t.Helper()
	path := filepath.Join("testdata", "raw", filename)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture not present: %v (run cmd/mccapture to regenerate)", err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	return frame
}

// TestIngestFirmwareFrame verifies that an 11 03 frame populates
// State.Device with the correct model, firmware version, and serial
// number.
func TestIngestFirmwareFrame(t *testing.T) {
	frame := parseFixture(t, "032_1103_len0037.sysex")

	state := model.NewState()
	ingestFrame(&state, frame, silentLogger)

	if state.Device.Model != 8 {
		t.Errorf("Model = %d, want 8", state.Device.Model)
	}
	wantFW := model.Version{Major: 3, Minor: 13, Patch: 6}
	if state.Device.Firmware != wantFW {
		t.Errorf("Firmware = %v, want %v", state.Device.Firmware, wantFW)
	}
	wantSerial := [4]byte{0x77, 0x4D, 0x26, 0x15}
	if state.Device.Serial != wantSerial {
		t.Errorf("Serial = % X, want % X", state.Device.Serial, wantSerial)
	}
}

// TestIngestPresetFrame verifies that an 06 01 frame populates
// State.Bank with the decoded preset and sets CurrentBank to the
// frame's bank index.
func TestIngestPresetFrame(t *testing.T) {
	frame := parseFixture(t, "002_0601_len1032.sysex")

	state := model.NewState()
	ingestFrame(&state, frame, silentLogger)

	// The captured frame is whichever preset was current during
	// mccapture's run. We don't know the exact bank index a
	// priori, but we can assert structural invariants.
	if state.CurrentBank < 0 || state.CurrentBank > 127 {
		t.Errorf("CurrentBank = %d, want 0..127", state.CurrentBank)
	}
	if state.Bank.BankNumber != state.CurrentBank {
		t.Errorf("Bank.BankNumber (%d) != CurrentBank (%d)",
			state.Bank.BankNumber, state.CurrentBank)
	}
	// The captured preset should have its BankNum populated by the
	// bank-index fix we made to DecodePresetFrame.
	preset := state.Bank.PresetArray[0]
	if preset.BankNum != state.CurrentBank {
		t.Errorf("preset[0].BankNum = %d, want %d", preset.BankNum, state.CurrentBank)
	}
}

// TestIngestBankNamesFrame verifies that a 03 20 frame populates
// State.BankNames at the row-tagged indices.
func TestIngestMidiChannelsFrame(t *testing.T) {
	frame := parseFixture(t, "004_0320_len0738.sysex")

	state := model.NewState()
	ingestFrame(&state, frame, silentLogger)

	// The test device names MIDI channel 2 "Quad Cortex Mini".
	if got := state.MidiChannels[1].Name; got != "Quad Cortex Mini" {
		t.Errorf("channel 2 name: got %q, want %q", got, "Quad Cortex Mini")
	}

	// The raw payload should also be stashed for round-trip fidelity.
	key := uint16(0x03)<<8 | uint16(0x20)
	if _, ok := state.Raw[key]; !ok {
		t.Error("expected raw payload to be stashed for 03 20")
	}
}

// TestIngestControllerSettingsFrames feeds the seven remapped
// 03 22–03 28 captures through ingestFrame and verifies each
// populates its typed State field (and is no longer stashed in Raw).
func TestIngestControllerSettingsFrames(t *testing.T) {
	state := model.NewState()
	for _, name := range []string{
		"012_0322_len0156.sysex",
		"029_0323_len0082.sysex",
		"007_0324_len0051.sysex",
		"009_0325_len0163.sysex",
		"010_0326_len0067.sysex",
		"030_0327_len0242.sysex",
		"031_0328_len0082.sysex",
	} {
		ingestFrame(&state, parseFixture(t, name), silentLogger)
	}

	if state.BankArrangement.NumBanksUsed != 127 {
		t.Errorf("BankArrangement.NumBanksUsed: got %d, want 127", state.BankArrangement.NumBanksUsed)
	}
	if len(state.Omniports) != 4 {
		t.Errorf("Omniports: got %d entries, want 4", len(state.Omniports))
	}
	if len(state.WaveformEngines) != 8 {
		t.Errorf("WaveformEngines: got %d entries, want 8", len(state.WaveformEngines))
	}
	if len(state.SequencerEngines) != 8 {
		t.Errorf("SequencerEngines: got %d entries, want 8", len(state.SequencerEngines))
	}
	if len(state.ScrollCounters) != 16 {
		t.Errorf("ScrollCounters: got %d entries, want 16", len(state.ScrollCounters))
	}
	if len(state.ResistorLadder) != 16 {
		t.Errorf("ResistorLadder: got %d entries, want 16", len(state.ResistorLadder))
	}
	// All 16 event slots are empty rules on the test device; spot-check
	// the decode ran by checking the scroll counters' default range,
	// which is non-zero on the wire.
	for i, c := range state.ScrollCounters {
		if c.Max != 127 {
			t.Errorf("ScrollCounters[%d].Max: got %d, want 127", i, c.Max)
		}
	}
	for _, cmd2 := range []uint16{0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28} {
		if _, ok := state.Raw[0x0300|cmd2]; ok {
			t.Errorf("frame 03 %02X unexpectedly stashed in Raw", cmd2)
		}
	}
}

// TestIngestUnknownFrameGoesToRaw verifies that a frame type we
// don't decode is stored in the Raw passthrough map.
func TestIngestUnknownFrameGoesToRaw(t *testing.T) {
	// Synthesize a frame with a command the SDK has no case for, so
	// it exercises the default path. (03 29 was used here before it
	// was identified as MIDI clock slots.)
	raw := sysex.Build(0x05, 0x55, sysex.NoArgs, []byte{1, 2, 3})
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatalf("parse synthetic frame: %v", err)
	}

	state := model.NewState()
	ingestFrame(&state, frame, silentLogger)

	key := uint16(frame.Cmd1)<<8 | uint16(frame.Cmd2)
	stored, ok := state.Raw[key]
	if !ok {
		t.Fatalf("expected Raw[%04X] to be populated", key)
	}
	if len(stored) != len(frame.Payload) {
		t.Errorf("Raw[%04X] has %d bytes, want %d", key, len(stored), len(frame.Payload))
	}
}

// TestIngestBankSwitchResetsBank verifies the "bank switch clears
// cross-bank contamination" behavior: when a preset frame arrives
// for a different bank than the one currently in State.Bank, the
// old Bank data is cleared so presets from different banks can't
// accidentally coexist in one Bank struct.
func TestIngestBankSwitchResetsBank(t *testing.T) {
	state := model.NewState()

	// Simulate a preset frame for bank 0, preset 0.
	state.Bank.BankNumber = 0
	state.Bank.PresetArray[5] = model.Preset{BankNum: 0, PresetNum: 5, ShortName: "from-bank-0"}
	state.CurrentBank = 0

	// Now ingest a preset frame for a DIFFERENT bank. We
	// synthesize one by encoding a minimal preset.
	newPreset := model.Preset{BankNum: 1, PresetNum: 3, ShortName: "from-bank-1"}
	payload := sysex.EncodePresetFrame(newPreset)
	frameBytes := sysex.Build(0x06, 0x01, sysex.NoArgs, payload)
	frame, err := sysex.Parse(frameBytes)
	if err != nil {
		t.Fatalf("parse synthesized frame: %v", err)
	}

	ingestFrame(&state, frame, silentLogger)

	// After ingestion, Bank should be a fresh Bank{BankNumber:1}
	// with only preset 3 populated and preset 5 wiped.
	if state.Bank.BankNumber != 1 {
		t.Errorf("Bank.BankNumber = %d, want 1", state.Bank.BankNumber)
	}
	if state.CurrentBank != 1 {
		t.Errorf("CurrentBank = %d, want 1", state.CurrentBank)
	}
	if state.Bank.PresetArray[3].ShortName != "from-bank-1" {
		t.Errorf("preset[3].ShortName = %q, want %q",
			state.Bank.PresetArray[3].ShortName, "from-bank-1")
	}
	if state.Bank.PresetArray[5].ShortName != "" {
		t.Errorf("preset[5] should be cleared but is %q",
			state.Bank.PresetArray[5].ShortName)
	}
}
