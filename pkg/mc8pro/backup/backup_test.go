package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

// mockClient implements the Client interface for testing without a
// live device. It returns a pre-built state with known values.
type mockClient struct {
	state model.State
	dev   model.Device
	// written tracks calls to the write methods.
	writtenPresets    []writeCall
	writtenExpPresets []writeCall
	writtenBankCfg    []int
	restoredBank      *model.Bank
}

type writeCall struct {
	bank, num int
}

func (m *mockClient) SelectBank(_ context.Context, bank int) error {
	m.state.CurrentBank = bank
	m.state.Bank.BankNumber = bank
	return nil
}

func (m *mockClient) ReadBank(_ context.Context) error { return nil }

func (m *mockClient) RestoreBank(_ context.Context, bank model.Bank) error {
	m.restoredBank = &bank
	return nil
}

func (m *mockClient) WritePreset(_ context.Context, bank, num int, _ model.Preset) error {
	m.writtenPresets = append(m.writtenPresets, writeCall{bank, num})
	return nil
}

func (m *mockClient) WriteExpPreset(_ context.Context, bank, num int, _ model.Preset) error {
	m.writtenExpPresets = append(m.writtenExpPresets, writeCall{bank, num})
	return nil
}

func (m *mockClient) WriteBankConfig(_ context.Context, bank int, _ model.Bank) error {
	m.writtenBankCfg = append(m.writtenBankCfg, bank)
	return nil
}

func (m *mockClient) Device() model.Device { return m.dev }
func (m *mockClient) State() model.State   { return m.state.Clone() }

func newMockClient() *mockClient {
	state := model.NewState()
	state.Bank.BankNumber = 0
	state.Bank.BankName = "Test Bank"
	state.Bank.PresetArray[0].ShortName = "Overdrive"
	state.Bank.PresetArray[0].MsgArray[0] = model.Message{
		M: 0, Type: 2, Channel: 2, Action: 1,
		Data: [18]int{35, 64},
	}
	return &mockClient{
		state: state,
		dev:   model.Device{Model: 8, Firmware: model.Version{Major: 3, Minor: 13, Patch: 6}},
	}
}

func TestExportBank(t *testing.T) {
	mc := newMockClient()
	ctx := context.Background()

	dump, err := ExportBank(ctx, mc, 0)
	if err != nil {
		t.Fatalf("ExportBank: %v", err)
	}

	if dump.DumpType != "singleBank" {
		t.Errorf("DumpType = %q, want singleBank", dump.DumpType)
	}
	if dump.DeviceModel != 8 {
		t.Errorf("DeviceModel = %d, want 8", dump.DeviceModel)
	}
	if dump.Data.Bank == nil {
		t.Fatal("Data.Bank is nil")
	}
	if dump.Data.Bank.BankName != "Test Bank" {
		t.Errorf("BankName = %q, want %q", dump.Data.Bank.BankName, "Test Bank")
	}
	if dump.Data.Bank.PresetArray[0].ShortName != "Overdrive" {
		t.Errorf("Preset[0].ShortName = %q, want %q",
			dump.Data.Bank.PresetArray[0].ShortName, "Overdrive")
	}
}

func TestImportBank(t *testing.T) {
	mc := newMockClient()
	ctx := context.Background()

	dump := model.Dump{
		DumpType: "singleBank",
		Data:     model.DumpData{Bank: &mc.state.Bank},
	}

	if err := ImportBank(ctx, mc, 5, dump); err != nil {
		t.Fatalf("ImportBank: %v", err)
	}

	// Should have called RestoreBank with bank number 5.
	if mc.restoredBank == nil {
		t.Fatal("RestoreBank was not called")
	}
	if mc.restoredBank.BankNumber != 5 {
		t.Errorf("RestoreBank bank number = %d, want 5", mc.restoredBank.BankNumber)
	}
	if mc.restoredBank.BankName != "Test Bank" {
		t.Errorf("RestoreBank bank name = %q, want %q", mc.restoredBank.BankName, "Test Bank")
	}
}

func TestImportBankRejectsNilBank(t *testing.T) {
	mc := newMockClient()
	ctx := context.Background()

	dump := model.Dump{DumpType: "allBanks"}
	err := ImportBank(ctx, mc, 0, dump)
	if err == nil {
		t.Fatal("expected error for nil bank data")
	}
}

func TestWriteReadFile(t *testing.T) {
	mc := newMockClient()
	ctx := context.Background()

	dump, err := ExportBank(ctx, mc, 0)
	if err != nil {
		t.Fatalf("ExportBank: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test-backup.json")

	if err := WriteFile(path, dump); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if loaded.DumpType != dump.DumpType {
		t.Errorf("DumpType = %q, want %q", loaded.DumpType, dump.DumpType)
	}
	if loaded.Data.Bank == nil {
		t.Fatal("loaded Data.Bank is nil")
	}
	if loaded.Data.Bank.BankName != "Test Bank" {
		t.Errorf("loaded BankName = %q, want %q",
			loaded.Data.Bank.BankName, "Test Bank")
	}

	// Verify the file is valid JSON that the editor could parse.
	raw, _ := os.ReadFile(path)
	var check map[string]any
	if err := json.Unmarshal(raw, &check); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if check["dumpType"] != "singleBank" {
		t.Errorf("JSON dumpType = %v, want singleBank", check["dumpType"])
	}
}
