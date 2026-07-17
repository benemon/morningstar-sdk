//go:build integration

package mc8pro_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

// TestIntegrationExternalAPI exercises the official 0x70 external
// SysEx API against real hardware. All writes use the TEMPORARY
// (non-save) flag, so nothing is committed to flash: the device's
// own bank-change revert restores original state.
//
// VISUAL CONFIRMATION: partway through, the LCD should show
// "SDK EXT TEST" for about a second, and preset A's label should
// briefly read "EXTTEST" before the bank up/down cycle restores it.
func TestIntegrationExternalAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{Logger: integrationLogger()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	state := client.State()
	t.Logf("session on bank %d %q", state.CurrentBank, state.Bank.BankName)

	t.Run("GetControllerInfo", func(t *testing.T) {
		info, err := client.GetControllerInfo(ctx)
		if err != nil {
			t.Fatalf("GetControllerInfo: %v", err)
		}
		t.Logf("controller info: %+v", info)
		if info.Model != 8 {
			t.Errorf("model: got %d, want 8 (MC8 Pro)", info.Model)
		}
		dev := client.Device()
		if info.Firmware[0] != int(dev.Firmware.Major) || info.Firmware[1] != int(dev.Firmware.Minor) {
			t.Errorf("firmware: got %v, want %d.%d.x from session open",
				info.Firmware, dev.Firmware.Major, dev.Firmware.Minor)
		}
		if info.MessagesPerPreset != 32 {
			t.Errorf("messages per preset: got %d, want 32", info.MessagesPerPreset)
		}
		if info.ShortNameLen != 32 || info.BankNameLen != 32 {
			t.Errorf("name sizes: got short=%d bank=%d, want 32/32",
				info.ShortNameLen, info.BankNameLen)
		}
	})

	t.Run("GetBankName", func(t *testing.T) {
		name, err := client.GetBankName(ctx)
		if err != nil {
			t.Fatalf("GetBankName: %v", err)
		}
		if name != state.Bank.BankName {
			t.Errorf("bank name: external API %q, session dump %q", name, state.Bank.BankName)
		}
	})

	t.Run("GetPresetNames", func(t *testing.T) {
		short, err := client.GetPresetShortName(ctx, 0)
		if err != nil {
			t.Fatalf("GetPresetShortName: %v", err)
		}
		if short != state.Bank.PresetArray[0].ShortName {
			t.Errorf("preset A short name: external API %q, session dump %q",
				short, state.Bank.PresetArray[0].ShortName)
		}
		long, err := client.GetPresetLongName(ctx, 0)
		if err != nil {
			t.Fatalf("GetPresetLongName: %v", err)
		}
		if long != state.Bank.PresetArray[0].LongName {
			t.Errorf("preset A long name: external API %q, session dump %q",
				long, state.Bank.PresetArray[0].LongName)
		}
	})

	t.Run("GetPresetToggles", func(t *testing.T) {
		states, err := client.GetPresetToggles(ctx)
		if err != nil {
			t.Fatalf("GetPresetToggles: %v", err)
		}
		t.Logf("toggle states (%d presets): %v", len(states), states)
		if len(states) != 24 {
			t.Errorf("toggle state count: got %d, want 24", len(states))
		}
	})

	t.Run("DisplayMessage", func(t *testing.T) {
		if err := client.DisplayMessage(ctx, "SDK EXT TEST", time.Second); err != nil {
			t.Fatalf("DisplayMessage: %v", err)
		}
		// Give the device its display second before the next command.
		time.Sleep(1200 * time.Millisecond)
	})

	t.Run("TemporaryNameOverride", func(t *testing.T) {
		original := state.Bank.PresetArray[0].ShortName

		if err := client.SetPresetShortName(ctx, 0, "EXTTEST", false); err != nil {
			t.Fatalf("SetPresetShortName (temporary): %v", err)
		}
		got, err := client.GetPresetShortName(ctx, 0)
		if err != nil {
			t.Fatalf("GetPresetShortName after override: %v", err)
		}
		if got != "EXTTEST" {
			t.Errorf("after temporary override: got %q, want EXTTEST", got)
		}

		// A temporary CC write alongside it, to exercise the message
		// function. Slot 15 (last external-API slot); reverts with
		// the name on bank change.
		msg := mc8pro.Message{
			Type:    model.MsgTypeCC,
			Action:  model.ActionPress,
			Toggle:  model.TogglePosBoth,
			Channel: 1,
		}
		msg.Data[0] = 99 // CC number
		msg.Data[1] = 77 // CC value
		if err := client.SetPresetMessage(ctx, 0, 15, msg, false); err != nil {
			t.Fatalf("SetPresetMessage (temporary): %v", err)
		}

		// Bank up then back down: the documented revert trigger for
		// temporary overrides, and it exercises the navigation
		// functions. Generous settles — bank changes make the device
		// dump state, and rushing it can crash the firmware.
		if err := client.BankUp(ctx); err != nil {
			t.Fatalf("BankUp: %v", err)
		}
		time.Sleep(1500 * time.Millisecond)
		if err := client.BankDown(ctx); err != nil {
			t.Fatalf("BankDown: %v", err)
		}
		time.Sleep(1500 * time.Millisecond)

		got, err = client.GetPresetShortName(ctx, 0)
		if err != nil {
			t.Fatalf("GetPresetShortName after revert: %v", err)
		}
		if got != original {
			t.Errorf("after bank-change revert: got %q, want original %q", got, original)
		}
	})
}

// bankCycle bounces the device one bank up and back down, which is
// the documented revert trigger for temporary external-API overrides.
// Generous settles: bank changes make the device dump state, and
// rushing it can crash the firmware.
func bankCycle(ctx context.Context, t *testing.T, client *mc8pro.Client) {
	t.Helper()
	if err := client.BankUp(ctx); err != nil {
		t.Fatalf("BankUp: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	if err := client.BankDown(ctx); err != nil {
		t.Fatalf("BankDown: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
}

// optionFields extracts the preset fields covered by SetPresetOptions
// for comparison.
func optionFields(p mc8pro.Preset) map[string]int {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	return map[string]int{
		"ToToggle":              b(p.ToToggle),
		"ToBlink":               b(p.ToBlink),
		"ToMsgScroll":           b(p.ToMsgScroll),
		"ToggleGroup":           p.ToggleGroup,
		"LedColor":              p.LedColor,
		"LedToggleColor":        p.LedToggleColor,
		"LedShiftColor":         p.LedShiftColor,
		"NameColor":             p.NameColor,
		"NameToggleColor":       p.NameToggleColor,
		"NameShiftColor":        p.NameShiftColor,
		"BackgroundColor":       p.BackgroundColor,
		"ToggleBackgroundColor": p.ToggleBackgroundColor,
		"ShiftBackgroundColor":  p.ShiftBackgroundColor,
	}
}

// TestIntegrationExternalAPIGaps exercises the external-API paths the
// first hardware run left uncovered: the toggle/long/bank name
// writes, TogglePage, SetPresetOptions (including the ExtColorKeep
// 0x7F leave-unchanged assumption, where the PDF prints an impossible
// "F7"), and the save-to-flash flag. Flash writes are limited to
// preset 0's name and options, and every one is explicitly restored
// to its original value before the test ends.
func TestIntegrationExternalAPIGaps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{Logger: integrationLogger()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	// Ground truth: a fresh full-bank read from flash.
	if err := client.ReadBank(ctx); err != nil {
		t.Fatalf("ReadBank: %v", err)
	}
	original := client.State().Bank.PresetArray[0]
	originalBankName := client.State().Bank.BankName
	// The device stays busy briefly after streaming a backup dump;
	// writes sent into that window are acked but applied late.
	time.Sleep(1 * time.Second)
	t.Logf("bank %q preset A %q, option fields: %v",
		originalBankName, original.ShortName, optionFields(original))

	// settle allows the device to APPLY an acked write before a
	// verifying read: the ack means accepted, not applied.
	settle := func() { time.Sleep(300 * time.Millisecond) }

	t.Run("TemporaryToggleLongBankNames", func(t *testing.T) {
		if err := client.SetPresetToggleName(ctx, 0, "EXTTOG", false); err != nil {
			t.Fatalf("SetPresetToggleName: %v", err)
		}
		settle()
		if got, _ := client.GetPresetToggleName(ctx, 0); got != "EXTTOG" {
			t.Errorf("toggle name after override: got %q, want EXTTOG", got)
		}
		if err := client.SetPresetLongName(ctx, 0, "EXTLONG", false); err != nil {
			t.Fatalf("SetPresetLongName: %v", err)
		}
		settle()
		if got, _ := client.GetPresetLongName(ctx, 0); got != "EXTLONG" {
			t.Errorf("long name after override: got %q, want EXTLONG", got)
		}
		if err := client.SetBankName(ctx, "EXTBANK", false); err != nil {
			t.Fatalf("SetBankName: %v", err)
		}
		settle()
		if got, _ := client.GetBankName(ctx); got != "EXTBANK" {
			t.Errorf("bank name after override: got %q, want EXTBANK", got)
		}

		bankCycle(ctx, t, client)

		if got, _ := client.GetPresetToggleName(ctx, 0); got != original.ToggleName {
			t.Errorf("toggle name after revert: got %q, want %q", got, original.ToggleName)
		}
		if got, _ := client.GetPresetLongName(ctx, 0); got != original.LongName {
			t.Errorf("long name after revert: got %q, want %q", got, original.LongName)
		}
		if got, _ := client.GetBankName(ctx); got != originalBankName {
			t.Errorf("bank name after revert: got %q, want %q", got, originalBankName)
		}
	})

	t.Run("TogglePage", func(t *testing.T) {
		// No page-state read exists, so this is acceptance-only: the
		// device must not reject the frames. Three toggles cycle
		// pages 1→2→3→1 on an unlimited bank.
		// VISUAL CONFIRMATION: the LCD page indicator cycles.
		for i := 0; i < 3; i++ {
			if err := client.TogglePage(ctx); err != nil {
				t.Fatalf("TogglePage %d: %v", i+1, err)
			}
			time.Sleep(800 * time.Millisecond)
		}
	})

	t.Run("TemporaryOptionsAndColorKeep", func(t *testing.T) {
		// op2 05 is TEMPORARY-ONLY on fw 3.13.6 (the save flag is
		// ineffective at every plausible opcode position — hardware
		// sweep 2026-07-17), so this verifies the write and the
		// ExtColorKeep leave-unchanged semantics via ReadBank, which
		// reads LIVE state (reflects temporary overrides): change
		// ONE flag and ONE color; every other option field must be
		// untouched; a bank cycle must revert everything.
		newBlink := !original.ToBlink
		newLed := 9 // red
		if original.LedColor == 9 {
			newLed = 4 // yellow
		}
		err := client.SetPresetOptions(ctx, 0, mc8pro.PresetOptions{
			ToBlink:      &newBlink,
			Pos1LedColor: &newLed,
		}, false)
		if err != nil {
			t.Fatalf("SetPresetOptions (temporary): %v", err)
		}
		if err := client.ReadBank(ctx); err != nil {
			t.Fatalf("ReadBank after temp options: %v", err)
		}
		p := client.State().Bank.PresetArray[0]
		if p.ToBlink != newBlink {
			t.Errorf("ToBlink override not applied: got %v, want %v", p.ToBlink, newBlink)
		}
		// Color values pass through a firmware translation layer, so
		// the stored index differs from the sent one (sending 9
		// stores 51 on fw 3.13.6). Assert only that the write landed
		// on the right field.
		if p.LedColor == original.LedColor {
			t.Errorf("Pos1LedColor override not applied: still %d", p.LedColor)
		} else {
			t.Logf("color translation: sent %d, device stored %d", newLed, p.LedColor)
		}
		// Keep-semantics: everything else must be untouched.
		want := optionFields(original)
		got := optionFields(p)
		for k, v := range want {
			if k == "ToBlink" || k == "LedColor" {
				continue
			}
			if got[k] != v {
				t.Errorf("keep failed for %s: got %d, want %d (ExtColorKeep assumption?)", k, got[k], v)
			}
		}

		bankCycle(ctx, t, client)
		if err := client.ReadBank(ctx); err != nil {
			t.Fatalf("ReadBank after revert: %v", err)
		}
		if got := optionFields(client.State().Bank.PresetArray[0]); !reflect.DeepEqual(got, optionFields(original)) {
			t.Errorf("revert mismatch:\ngot  %v\nwant %v", got, optionFields(original))
		}
	})

	t.Run("SaveShortNamePath", func(t *testing.T) {
		// Save-to-flash for the op4-flag functions: a SAVED name must
		// SURVIVE a bank change (temporary ones revert). Restored to
		// the original immediately after.
		if err := client.SetPresetShortName(ctx, 0, "EXTSAVE", true); err != nil {
			t.Fatalf("SetPresetShortName (save): %v", err)
		}
		time.Sleep(1500 * time.Millisecond)
		bankCycle(ctx, t, client)
		if got, _ := client.GetPresetShortName(ctx, 0); got != "EXTSAVE" {
			t.Errorf("saved name did not persist bank change: got %q, want EXTSAVE", got)
		}

		if err := client.SetPresetShortName(ctx, 0, original.ShortName, true); err != nil {
			t.Fatalf("SetPresetShortName (restore): %v", err)
		}
		time.Sleep(1500 * time.Millisecond)
		bankCycle(ctx, t, client)
		if got, _ := client.GetPresetShortName(ctx, 0); got != original.ShortName {
			t.Errorf("restore failed: got %q, want %q — MANUAL FIX NEEDED on device", got, original.ShortName)
		}
	})
}
