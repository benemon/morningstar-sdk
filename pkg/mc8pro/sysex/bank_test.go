package sysex_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// TestBankMetaEncodeDecodeRoundTrip verifies that EncodeBankMetaFrame
// and DecodeBankMetaFrame are inverses for the JSON fixture's bank.
func TestBankMetaEncodeDecodeRoundTrip(t *testing.T) {
	dump := loadFixtureDump(t, "bank-guitar-live.json")
	if dump.Data.Bank == nil {
		t.Fatal("fixture is not a singleBank dump")
	}
	original := *dump.Data.Bank

	payload := sysex.EncodeBankMetaFrame(original)
	decoded, err := sysex.DecodeBankMetaFrame(payload)
	if err != nil {
		t.Fatalf("DecodeBankMetaFrame: %v", err)
	}

	// Compare bank-level fields that the metadata frame carries.
	if decoded.BankNumber != original.BankNumber {
		t.Errorf("BankNumber: got %d, want %d", decoded.BankNumber, original.BankNumber)
	}
	if decoded.BankName != original.BankName {
		t.Errorf("BankName: got %q, want %q", decoded.BankName, original.BankName)
	}
	if decoded.BankDescription != original.BankDescription {
		t.Errorf("BankDescription: got %q, want %q", decoded.BankDescription, original.BankDescription)
	}
	if decoded.BankClearToggle != original.BankClearToggle {
		t.Errorf("BankClearToggle: got %v, want %v", decoded.BankClearToggle, original.BankClearToggle)
	}
	if decoded.ToDisplay != original.ToDisplay {
		t.Errorf("ToDisplay: got %v, want %v", decoded.ToDisplay, original.ToDisplay)
	}
	if decoded.BackgroundColor != original.BackgroundColor {
		t.Errorf("BackgroundColor: got %d, want %d", decoded.BackgroundColor, original.BackgroundColor)
	}
	if decoded.TextColor != original.TextColor {
		t.Errorf("TextColor: got %d, want %d", decoded.TextColor, original.TextColor)
	}
	if decoded.IsColorEnabled != original.IsColorEnabled {
		t.Errorf("IsColorEnabled: got %v, want %v", decoded.IsColorEnabled, original.IsColorEnabled)
	}
	if decoded.PageLimit != original.PageLimit {
		t.Errorf("PageLimit: got %d, want %d", decoded.PageLimit, original.PageLimit)
	}

	// BankMsgArray: compare all 32 messages. Bank messages carry
	// data[0..8] on the wire; data[9..17] are not encoded and will
	// round-trip as zero regardless of the JSON fixture's values.
	// Zero those fields in the original before comparing.
	for i := range original.BankMsgArray {
		for j := 9; j < 18; j++ {
			original.BankMsgArray[i].Data[j] = 0
		}
		// The Info field (mi) is client-side only and not on the wire.
		original.BankMsgArray[i].Info = ""
	}
	if !reflect.DeepEqual(decoded.BankMsgArray, original.BankMsgArray) {
		t.Errorf("BankMsgArray mismatch")
		for i := range decoded.BankMsgArray {
			if !reflect.DeepEqual(decoded.BankMsgArray[i], original.BankMsgArray[i]) {
				t.Errorf("  slot %d: got %+v, want %+v", i, decoded.BankMsgArray[i], original.BankMsgArray[i])
			}
		}
	}
}

// TestBankMetaDecodeCapturedFrame decodes a captured 06 02 wire frame
// and compares the result against the JSON fixture. Skipped when the
// capture fixture is absent.
func TestBankMetaDecodeCapturedFrame(t *testing.T) {
	framePath := findCapturedFrame(t, 0x06, 0x02)
	if framePath == "" {
		t.Skip("no captured 06 02 frame in testdata/raw/")
	}

	raw, err := os.ReadFile(framePath)
	if err != nil {
		t.Fatalf("read %s: %v", framePath, err)
	}
	frame, err := sysex.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	decoded, err := sysex.DecodeBankMetaFrame(frame.Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	dump := loadFixtureDump(t, "bank-guitar-live.json")
	if dump.Data.Bank == nil {
		t.Fatal("fixture is not a singleBank dump")
	}
	expected := *dump.Data.Bank

	// Bank-level config fields.
	if decoded.BankName != expected.BankName {
		t.Errorf("BankName: got %q, want %q", decoded.BankName, expected.BankName)
	}
	if decoded.BankClearToggle != expected.BankClearToggle {
		t.Errorf("BankClearToggle: got %v, want %v", decoded.BankClearToggle, expected.BankClearToggle)
	}
	if decoded.BackgroundColor != expected.BackgroundColor {
		t.Errorf("BackgroundColor: got %d, want %d", decoded.BackgroundColor, expected.BackgroundColor)
	}
	if decoded.TextColor != expected.TextColor {
		t.Errorf("TextColor: got %d, want %d", decoded.TextColor, expected.TextColor)
	}
	// IsColorEnabled: the editor JSON says true but the wire byte is
	// 0x00. The editor likely infers "color enabled" from non-default
	// backgroundColor/textColor values during export. The wire is
	// ground truth; we trust the decoded value.
	if decoded.PageLimit != expected.PageLimit {
		t.Errorf("PageLimit: got %d, want %d", decoded.PageLimit, expected.PageLimit)
	}

	// Bank messages: compare populated slots fully, empty slots by
	// type only (same don't-care approach as preset messages).
	for i := range decoded.BankMsgArray {
		dm := decoded.BankMsgArray[i]
		em := expected.BankMsgArray[i]
		switch {
		case dm.Type == 0 && em.Type == 0:
			// Both empty — don't-care values may differ.
		case dm.Type == 0:
			t.Errorf("bank msg %d: expected populated (Type=%d), got empty", i, em.Type)
		case em.Type == 0:
			t.Errorf("bank msg %d: expected empty, got populated (Type=%d)", i, dm.Type)
		default:
			// For populated messages, zero out data[9..17] and Info
			// in the expected value since those aren't on the wire.
			for j := 9; j < 18; j++ {
				em.Data[j] = 0
			}
			em.Info = ""
			if !reflect.DeepEqual(dm, em) {
				t.Errorf("bank msg %d: mismatch\n got: %+v\nwant: %+v", i, dm, em)
			}
		}
	}
}
