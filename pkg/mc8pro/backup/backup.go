// Package backup provides JSON backup export and import for the
// Morningstar MC8 Pro. It is a thin layer on top of the mc8pro.Client
// — reads use Client.SelectBank/ReadBank, writes use
// Client.WritePreset/WriteBankConfig. No new protocol operations are
// introduced.
//
// The exported JSON files are compatible with the Morningstar web
// editor's import feature.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

// Client is the subset of mc8pro.Client that backup operations need.
// Using an interface here avoids an import cycle (backup → mc8pro →
// backup) and makes unit testing possible without a live device.
type Client interface {
	SelectBank(ctx context.Context, bank int) error
	ReadBank(ctx context.Context) error
	RestoreBank(ctx context.Context, bank model.Bank) error
	WritePreset(ctx context.Context, bank, presetNum int, p model.Preset) error
	WriteExpPreset(ctx context.Context, bank, expNum int, p model.Preset) error
	WriteBankConfig(ctx context.Context, bank int, b model.Bank) error
	Device() model.Device
	State() model.State
}

// ExportBank reads the specified bank from the device and wraps it in
// a Morningstar-compatible JSON Dump structure. The result can be
// marshalled to JSON and imported by the official editor.
func ExportBank(ctx context.Context, c Client, bankNum int) (model.Dump, error) {
	if err := c.SelectBank(ctx, bankNum); err != nil {
		return model.Dump{}, fmt.Errorf("backup: select bank %d: %w", bankNum, err)
	}

	state := c.State()
	dev := c.Device()

	return model.Dump{
		SchemaVersion: 1,
		DumpType:      "singleBank",
		DeviceModel:   dev.Model,
		DownloadDate:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Data: model.DumpData{
			Bank: &state.Bank,
		},
	}, nil
}

// ImportBank uploads a single-bank Dump to the device using the
// restore protocol — the inverse of the backup dump. The device
// drives the pace via handshake ACKs, so no timing heuristics are
// needed. This is the same mechanism the official editor uses when
// you click "Good to go! Click here to upload."
//
// The bank is addressed by bankNum, regardless of what BankNumber
// is set in the dump data.
func ImportBank(ctx context.Context, c Client, bankNum int, dump model.Dump) error {
	if dump.Data.Bank == nil {
		return fmt.Errorf("backup: dump has no bank data (dumpType=%q)", dump.DumpType)
	}

	bank := *dump.Data.Bank
	bank.BankNumber = bankNum

	return c.RestoreBank(ctx, bank)
}

// WriteFile marshals a Dump to JSON and writes it to path.
func WriteFile(path string, dump model.Dump) error {
	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// ReadFile reads a Morningstar JSON backup from path and returns the
// parsed Dump.
func ReadFile(path string) (model.Dump, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Dump{}, fmt.Errorf("backup: read %s: %w", path, err)
	}
	var dump model.Dump
	if err := json.Unmarshal(data, &dump); err != nil {
		return model.Dump{}, fmt.Errorf("backup: unmarshal %s: %w", path, err)
	}
	return dump, nil
}
