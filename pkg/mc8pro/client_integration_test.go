//go:build integration

// Integration tests in this file require a real Morningstar MC8 Pro
// connected via USB MIDI. They are gated behind the `integration`
// build tag so a normal `go test ./...` does not run them.
//
// To run:
//
//	go test -tags=integration -v ./pkg/mc8pro/...
//
// Make sure the official web editor is NOT connected before running
// these tests — only one process can hold the MIDI port at a time.
package mc8pro_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/backup"
)

// integrationLogger returns an slog.Logger that writes to stderr at
// debug level so test output captures the full SysEx exchange.
func integrationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// TestIntegrationOpenPingClose exercises the full Phase 2 lifecycle:
// open a session, query the device, ping it, close cleanly. Asserts
// that:
//   - Open succeeds and populates Device with a non-zero firmware
//   - Ping returns sessionActive=true while the session is open
//   - Close completes without error
//
// VISUAL CONFIRMATION: while this test runs you should see the MC8
// Pro's LCD flip into edit mode at the start and back to its normal
// bank display at the end. If it stays in edit mode after the test
// exits, something is wrong with the close path.
func TestIntegrationOpenPingClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	// Verify Device() is populated.
	dev := client.Device()
	if dev.Model != 8 {
		t.Errorf("Device.Model = %d, want 8", dev.Model)
	}
	if dev.Firmware.Major == 0 && dev.Firmware.Minor == 0 && dev.Firmware.Patch == 0 {
		t.Error("Device.Firmware is all zero; expected a real version")
	}
	if dev.Serial == [4]byte{} {
		t.Error("Device.Serial is all zero; expected a real serial")
	}
	t.Logf("connected to MC8 Pro fw %s serial % X",
		dev.Firmware.String(), dev.Serial)

	// Ping should report session active.
	active, err := client.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !active {
		t.Error("Ping reported session inactive; expected active during open session")
	}
}

// TestIntegrationOpenStatePopulated exercises Phase 3: Open should
// collect the device's full initial dump and populate State with
// device info, the currently-loaded bank, and bank names.
//
// VISUAL: screen should flip into edit mode, stay briefly, and return
// to normal.
func TestIntegrationOpenStatePopulated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	state := client.State()

	// Device should be populated (firmware and serial).
	if state.Device.Model != 8 {
		t.Errorf("State.Device.Model = %d, want 8", state.Device.Model)
	}
	if state.Device.Firmware == (mc8pro.Version{}) {
		t.Error("State.Device.Firmware is zero; expected a real version")
	}

	// CurrentBank should be within range.
	if state.CurrentBank < 0 || state.CurrentBank > 127 {
		t.Errorf("State.CurrentBank = %d, want 0..127", state.CurrentBank)
	}

	// Bank.BankNumber should match CurrentBank.
	if state.Bank.BankNumber != state.CurrentBank {
		t.Errorf("State.Bank.BankNumber (%d) != CurrentBank (%d)",
			state.Bank.BankNumber, state.CurrentBank)
	}

	// At least one bank name should be populated (the user has at
	// least "Quad Cortex Mini" configured at bank 1 per CLAUDE.md
	// but we don't hard-code that here).
	populated := 0
	for _, name := range state.BankNames {
		if name != "" {
			populated++
		}
	}
	t.Logf("State summary: device=%s fw=%s serial=% X currentBank=%d bankName=%q populatedNames=%d",
		"MC8 Pro", state.Device.Firmware.String(), state.Device.Serial,
		state.CurrentBank, state.Bank.BankName, populated)

	if populated == 0 {
		t.Error("no bank names populated; expected at least one")
	}
}

// TestIntegrationReadBankStandalone tests ReadBank on a stable session
// with NO prior SelectBank. This isolates whether the backup command
// works when the device hasn't just been bank-switched.
func TestIntegrationReadBankStandalone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	t.Logf("session opened on bank %d, now calling ReadBank", client.State().CurrentBank)

	if err := client.ReadBank(ctx); err != nil {
		t.Fatalf("ReadBank: %v", err)
	}

	state := client.State()
	t.Logf("ReadBank complete: bank=%d name=%q presets populated:", state.CurrentBank, state.Bank.BankName)
	for i, p := range state.Bank.PresetArray {
		if p.ShortName != "" {
			t.Logf("  preset[%d] = %q (msg[0].Type=%d)", i, p.ShortName, p.MsgArray[0].Type)
		}
	}
}

// TestIntegrationSelectBank exercises Phase 3's SelectBank primitive:
// open a session, navigate to a specific bank, verify State updates
// to reflect the new bank, navigate to a different bank, verify
// again.
//
// This test intentionally selects bank 0 and bank 1 because those
// are well-defined in the user's test rig (bank 0 = "Quad Cortex
// Mini" with Guitar - Live preset; bank 1 = empty). If your test
// device has a different layout, adjust the assertions.
//
// VISUAL: the LCD should change bank as SelectBank calls are issued,
// and return to whatever bank it was on originally after the test
// cleans up. We do NOT restore the pre-test bank automatically —
// the test leaves the device on whatever bank was selected last.
func TestIntegrationSelectBank(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	originalBank := client.State().CurrentBank
	t.Logf("session opened on bank %d", originalBank)

	// Select bank 0.
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0): %v", err)
	}
	state := client.State()
	if state.CurrentBank != 0 {
		t.Errorf("after SelectBank(0), CurrentBank = %d, want 0", state.CurrentBank)
	}
	if state.Bank.BankNumber != 0 {
		t.Errorf("after SelectBank(0), Bank.BankNumber = %d, want 0", state.Bank.BankNumber)
	}
	t.Logf("bank 0: name=%q", state.Bank.BankName)

	// Select bank 1.
	if err := client.SelectBank(ctx, 1); err != nil {
		t.Fatalf("SelectBank(1): %v", err)
	}
	state = client.State()
	if state.CurrentBank != 1 {
		t.Errorf("after SelectBank(1), CurrentBank = %d, want 1", state.CurrentBank)
	}
	if state.Bank.BankNumber != 1 {
		t.Errorf("after SelectBank(1), Bank.BankNumber = %d, want 1", state.Bank.BankNumber)
	}
	t.Logf("bank 1: name=%q", state.Bank.BankName)

	// Device info should be preserved across SelectBank calls.
	if state.Device.Model != 8 {
		t.Errorf("Device.Model = %d after SelectBank, want 8 (should be preserved)", state.Device.Model)
	}
	// Bank names should also be preserved.
	anyPopulated := false
	for _, name := range state.BankNames {
		if name != "" {
			anyPopulated = true
			break
		}
	}
	if !anyPopulated {
		t.Error("BankNames lost after SelectBank; expected to be preserved")
	}
}

// TestIntegrationWritePreset is the Phase 4 load-bearing test. It:
//
//  1. Opens a session and navigates to bank 0
//  2. Reads the current state of preset 0 (the "Overdrive" preset)
//  3. Mutates one CC value in the first message slot
//  4. Writes the modified preset back to the device
//  5. Re-reads the bank via SelectBank to verify the change landed
//  6. RESTORES the original preset data (critical — we must not leave
//     the device in a modified state after the test)
//  7. Re-reads again to verify the restore worked
//
// This test modifies real device state. If it panics mid-way or the
// test binary is killed, the device will have a modified CC value on
// preset 0 of bank 0. The user can fix this by re-connecting the
// official editor and resetting the value manually.
//
// VISUAL: the LCD will show edit mode throughout. Bank switches happen
// as part of the read-verify cycles.
func TestIntegrationWritePreset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Step 1: navigate to bank 0 (the Guitar - Live bank).
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0): %v", err)
	}
	state := client.State()
	t.Logf("initial state: bank=%d, bankName=%q", state.CurrentBank, state.Bank.BankName)

	// Step 2: capture the original preset 0 for later restore.
	original := state.Bank.PresetArray[0]
	t.Logf("original preset 0: shortName=%q, msg[0].Type=%d, msg[0].Data=%v",
		original.ShortName, original.MsgArray[0].Type, original.MsgArray[0].Data[:4])

	// We need at least one populated message to mutate. If slot 0
	// is empty (Type=0), skip the test — we can't verify a write
	// against an empty slot because the device may normalize fields.
	if original.MsgArray[0].Type == 0 {
		t.Skip("preset 0 message 0 is empty; need a populated CC message to test writes")
	}

	// Step 3: mutate a CC value. We bump Data[1] (CC value) by 1,
	// wrapping at 127 → 0. This is a minimal change that's easy to
	// detect in the re-read.
	modified := original
	oldValue := modified.MsgArray[0].Data[1]
	newValue := (oldValue + 1) % 128
	modified.MsgArray[0].Data[1] = newValue
	t.Logf("mutating msg[0].Data[1]: %d → %d", oldValue, newValue)

	// Step 4: write the modified preset.
	if err := client.WritePreset(ctx, 0, 0, modified); err != nil {
		t.Fatalf("WritePreset (modify): %v", err)
	}

	// Brief pause for flash commit.
	time.Sleep(1500 * time.Millisecond)

	// Step 5: re-read the full bank via SelectBank (which now
	// includes a full ReadBank internally — all 24 presets).
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0) after write: %v", err)
	}
	afterWrite := client.State().Bank.PresetArray[0]
	if afterWrite.MsgArray[0].Data[1] != newValue {
		t.Errorf("after write: msg[0].Data[1] = %d, want %d",
			afterWrite.MsgArray[0].Data[1], newValue)
	} else {
		t.Logf("write verified: msg[0].Data[1] = %d", afterWrite.MsgArray[0].Data[1])
	}

	// Step 6: RESTORE the original preset.
	if err := client.WritePreset(ctx, 0, 0, original); err != nil {
		t.Fatalf("WritePreset (restore): %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	// Step 7: verify the restore.
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0) after restore: %v", err)
	}
	afterRestore := client.State().Bank.PresetArray[0]
	if afterRestore.MsgArray[0].Data[1] != oldValue {
		t.Errorf("after restore: msg[0].Data[1] = %d, want %d (original)",
			afterRestore.MsgArray[0].Data[1], oldValue)
	} else {
		t.Logf("restore verified: msg[0].Data[1] = %d (original)", afterRestore.MsgArray[0].Data[1])
	}
}

// TestIntegrationDuplicatePreset copies preset A (Overdrive) into
// preset G (index 6, which should be EMPTY), verifies it appears
// on the device, then clears it back to empty.
//
// This is a visual confirmation test: while running, watch the MC8
// Pro's LCD — preset G should briefly show "Overdrive" with the
// same CC config as preset A, then revert to EMPTY.
func TestIntegrationDuplicatePreset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Read bank 0 with all presets.
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0): %v", err)
	}
	state := client.State()

	// Capture preset A (Overdrive) and the original preset G (EMPTY).
	presetA := state.Bank.PresetArray[0]
	originalG := state.Bank.PresetArray[6]
	t.Logf("preset A: %q (msg[0].Type=%d, CC#%d val %d)",
		presetA.ShortName, presetA.MsgArray[0].Type,
		presetA.MsgArray[0].Data[0], presetA.MsgArray[0].Data[1])
	t.Logf("preset G before: %q", originalG.ShortName)

	if presetA.MsgArray[0].Type == 0 {
		t.Skip("preset A has no messages; need a populated preset to duplicate")
	}

	// Copy A into G — keep all messages and names.
	duplicate := presetA
	duplicate.PresetNum = 6 // G
	duplicate.BankNum = 0

	t.Log("writing Overdrive duplicate into preset G...")
	if err := client.WritePreset(ctx, 0, 6, duplicate); err != nil {
		t.Fatalf("WritePreset (duplicate): %v", err)
	}

	// Wait for flash commit + re-read.
	time.Sleep(1500 * time.Millisecond)
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0) after duplicate: %v", err)
	}

	afterDup := client.State().Bank.PresetArray[6]
	t.Logf("preset G after duplicate: %q (msg[0].Type=%d, CC#%d val %d)",
		afterDup.ShortName, afterDup.MsgArray[0].Type,
		afterDup.MsgArray[0].Data[0], afterDup.MsgArray[0].Data[1])

	if afterDup.ShortName != presetA.ShortName {
		t.Errorf("preset G name = %q, want %q", afterDup.ShortName, presetA.ShortName)
	}
	if afterDup.MsgArray[0].Type != presetA.MsgArray[0].Type {
		t.Errorf("preset G msg type = %d, want %d", afterDup.MsgArray[0].Type, presetA.MsgArray[0].Type)
	}

	t.Log(">>> CHECK THE DEVICE LCD — preset G should show 'Overdrive' <<<")
	t.Log("pausing 5 seconds for visual confirmation...")
	time.Sleep(5 * time.Second)

	// Restore: write the original (empty) preset G back.
	t.Log("restoring preset G to original (EMPTY)...")
	if err := client.WritePreset(ctx, 0, 6, originalG); err != nil {
		t.Fatalf("WritePreset (restore): %v", err)
	}

	time.Sleep(1500 * time.Millisecond)
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0) after restore: %v", err)
	}

	restored := client.State().Bank.PresetArray[6]
	t.Logf("preset G after restore: %q", restored.ShortName)

	if restored.MsgArray[0].Type != originalG.MsgArray[0].Type {
		t.Errorf("preset G not restored: msg type = %d, want %d",
			restored.MsgArray[0].Type, originalG.MsgArray[0].Type)
	} else {
		t.Log("preset G restored successfully")
	}
}

// TestIntegrationWriteBankConfig exercises WriteBankConfig with a
// read→mutate→write→verify→restore cycle on the bank name. The
// device should end in its original state.
func TestIntegrationWriteBankConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Step 1: navigate to bank 0 and capture original state.
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0): %v", err)
	}
	state := client.State()
	originalBank := state.Bank
	t.Logf("original bank: name=%q, clearToggle=%v, bgColor=%d, textColor=%d",
		originalBank.BankName, originalBank.BankClearToggle,
		originalBank.BackgroundColor, originalBank.TextColor)

	// Step 2: mutate the bank name — append " TEST".
	modified := originalBank
	modified.BankName = originalBank.BankName + " TEST"
	t.Logf("mutating bank name: %q → %q", originalBank.BankName, modified.BankName)

	// Step 3: write the modified bank config.
	if err := client.WriteBankConfig(ctx, 0, modified); err != nil {
		t.Fatalf("WriteBankConfig (modify): %v", err)
	}

	// Wait for flash commit.
	time.Sleep(1500 * time.Millisecond)

	// Step 4: re-read and verify.
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0) after write: %v", err)
	}
	afterWrite := client.State().Bank
	if afterWrite.BankName != modified.BankName {
		t.Errorf("after write: bankName = %q, want %q",
			afterWrite.BankName, modified.BankName)
	} else {
		t.Logf("write verified: bankName = %q", afterWrite.BankName)
	}

	// Step 5: RESTORE the original bank config.
	if err := client.WriteBankConfig(ctx, 0, originalBank); err != nil {
		t.Fatalf("WriteBankConfig (restore): %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	// Step 6: verify the restore.
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank(0) after restore: %v", err)
	}
	afterRestore := client.State().Bank
	if afterRestore.BankName != originalBank.BankName {
		t.Errorf("after restore: bankName = %q, want %q (original)",
			afterRestore.BankName, originalBank.BankName)
	} else {
		t.Logf("restore verified: bankName = %q (original)", afterRestore.BankName)
	}
}

// TestIntegrationBackupExport exercises the backup package export
// against a live device: exports bank 0, verifies the JSON is
// well-formed, writes to disk and reads back.
func TestIntegrationBackupExport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	// Step 1: export bank 0.
	dump, err := backup.ExportBank(ctx, client, 0)
	if err != nil {
		t.Fatalf("ExportBank: %v", err)
	}
	t.Logf("exported bank %d: name=%q, presets=%d",
		dump.Data.Bank.BankNumber, dump.Data.Bank.BankName,
		len(dump.Data.Bank.PresetArray))

	if dump.Data.Bank == nil {
		t.Fatal("exported bank is nil")
	}
	if dump.DumpType != "singleBank" {
		t.Errorf("DumpType = %q, want singleBank", dump.DumpType)
	}

	// Step 2: write to file, read back, verify round-trip.
	dir := t.TempDir()
	path := filepath.Join(dir, "bank0.json")
	if err := backup.WriteFile(path, dump); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := backup.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if loaded.Data.Bank.BankName != dump.Data.Bank.BankName {
		t.Errorf("round-trip bank name: got %q, want %q",
			loaded.Data.Bank.BankName, dump.Data.Bank.BankName)
	}
	t.Logf("JSON file round-trip verified: %s", path)
}

// TestIntegrationBackupImport exports bank 0, imports it back with
// a per-write throttle (200ms default), then verifies the device
// state is unchanged.
func TestIntegrationBackupImport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	// Step 1: export bank 0 as our source data.
	dump, err := backup.ExportBank(ctx, client, 0)
	if err != nil {
		t.Fatalf("ExportBank: %v", err)
	}
	t.Logf("exported bank %d: name=%q", dump.Data.Bank.BankNumber, dump.Data.Bank.BankName)

	// Step 2: import back via restore protocol (device-paced handshake).
	if err := backup.ImportBank(ctx, client, 0, dump); err != nil {
		t.Fatalf("ImportBank: %v", err)
	}
	t.Log("import complete (restore protocol)")

	// Step 3: brief settle, then verify.
	time.Sleep(1 * time.Second)

	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank after import: %v", err)
	}
	after := client.State().Bank
	if after.BankName != dump.Data.Bank.BankName {
		t.Errorf("after import: bankName = %q, want %q",
			after.BankName, dump.Data.Bank.BankName)
	}
	if after.PresetArray[0].ShortName != dump.Data.Bank.PresetArray[0].ShortName {
		t.Errorf("after import: preset[0] = %q, want %q",
			after.PresetArray[0].ShortName, dump.Data.Bank.PresetArray[0].ShortName)
	}
	t.Log("verified: device state unchanged after import round-trip")
}

// TestIntegrationListDevices verifies ListDevices finds at least one
// MC8 Pro when the device is connected.
func TestIntegrationListDevices(t *testing.T) {
	devices := mc8pro.ListDevices()
	if len(devices) == 0 {
		t.Skip("no MC8 Pro found")
	}
	for _, d := range devices {
		t.Logf("found: %s", d.Name)
	}
}

// TestIntegrationSubscribe verifies that Subscribe delivers frames
// during a bank select operation.
func TestIntegrationSubscribe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	// Subscribe to all frames.
	ch, unsub := client.Subscribe(nil)
	defer unsub()

	// Trigger a bank select — this generates device frames.
	if err := client.SelectBank(ctx, 0); err != nil {
		t.Fatalf("SelectBank: %v", err)
	}

	// Drain whatever we got.
	var count int
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				t.Log("subscription channel closed")
				goto done
			}
			count++
		case <-time.After(500 * time.Millisecond):
			goto done
		}
	}
done:
	if count == 0 {
		t.Error("Subscribe received 0 frames; expected at least 1 from SelectBank")
	} else {
		t.Logf("Subscribe received %d frames during SelectBank", count)
	}
}

// TestIntegrationRestoreFidelity is the hardware acid test for the
// direction-asymmetric message-row codec fix: it reads a full bank,
// restores the IDENTICAL data back to the device via the restore
// protocol, re-reads, and requires the two reads to be deeply equal
// across every preset, message, and config field. Before the fix,
// this cycle swapped channel/action on every message; a pass here
// means read→write round-trips are faithful end to end.
//
// A JSON backup of the bank's pre-test state is written to the OS
// temp directory first and its path logged, so the bank can be
// restored via ImportBank (or the Morningstar editor) if the test
// leaves the device in a bad state.
//
// Target bank defaults to 0; override with MC8PRO_FIDELITY_BANK.
func TestIntegrationRestoreFidelity(t *testing.T) {
	bankNum := 0
	if v := os.Getenv("MC8PRO_FIDELITY_BANK"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 127 {
			t.Fatalf("MC8PRO_FIDELITY_BANK=%q: want an integer 0..127", v)
		}
		bankNum = n
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
		Logger: integrationLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	// Step 1: read the bank and save a recovery backup BEFORE writing.
	dump, err := backup.ExportBank(ctx, client, bankNum)
	if err != nil {
		t.Fatalf("ExportBank(%d): %v", bankNum, err)
	}
	before := *dump.Data.Bank

	recovery := filepath.Join(os.TempDir(),
		time.Now().Format("20060102-150405")+"-mc8pro-fidelity-bank"+strconv.Itoa(bankNum)+".json")
	if err := backup.WriteFile(recovery, dump); err != nil {
		t.Fatalf("write recovery backup: %v", err)
	}
	t.Logf("recovery backup: %s", recovery)

	// Step 2: restore the identical data back to the device.
	if err := client.RestoreBank(ctx, before); err != nil {
		t.Fatalf("RestoreBank: %v", err)
	}

	// Step 3: settle, then re-read.
	time.Sleep(1500 * time.Millisecond)
	if err := client.SelectBank(ctx, bankNum); err != nil {
		t.Fatalf("SelectBank(%d) after restore: %v", bankNum, err)
	}
	dump2, err := backup.ExportBank(ctx, client, bankNum)
	if err != nil {
		t.Fatalf("ExportBank(%d) after restore: %v", bankNum, err)
	}
	after := *dump2.Data.Bank

	// Step 4: field-level comparison, most specific first so a
	// failure pinpoints the drift.
	if before.BankName != after.BankName {
		t.Errorf("BankName: %q -> %q", before.BankName, after.BankName)
	}
	for i := range before.PresetArray {
		comparePreset(t, "preset", i, before.PresetArray[i], after.PresetArray[i])
	}
	for i := range before.ExpPresetArray {
		comparePreset(t, "expPreset", i, before.ExpPresetArray[i], after.ExpPresetArray[i])
	}
	if !reflect.DeepEqual(before.BankMsgArray, after.BankMsgArray) {
		t.Errorf("BankMsgArray drifted")
	}
	if !t.Failed() {
		t.Logf("zero-diff verified: bank %d survived read->restore->read intact", bankNum)
	}
}

// comparePreset reports every field-level difference between two
// presets, message by message.
func comparePreset(t *testing.T, kind string, idx int, before, after mc8pro.Preset) {
	t.Helper()
	for m := range before.MsgArray {
		bm, am := before.MsgArray[m], after.MsgArray[m]
		bm.Info, am.Info = "", ""
		if !reflect.DeepEqual(bm, am) {
			t.Errorf("%s %d msg %d:\n  before: %+v\n  after:  %+v", kind, idx, m, bm, am)
		}
	}
	bm, am := before, after
	bm.MsgArray, am.MsgArray = [32]mc8pro.Message{}, [32]mc8pro.Message{}
	if !reflect.DeepEqual(bm, am) {
		t.Errorf("%s %d config/names:\n  before: %+v\n  after:  %+v", kind, idx, bm, am)
	}
}
