package mc8pro

import (
	"bytes"
	"context"
	"testing"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// TestExternalFrameMatchesSpec pins the wire bytes of an external-API
// frame against the documented example (Controller Bank Up):
//
//	F0 00 21 24 id 00 70 00 00 00 00 00 00 00 00 00 cs F7
//
// from reference/sysex-api-external-applications.pdf. The standard
// Build path must produce exactly this shape — op7 (byte 12),
// transaction ID (byte 13), and the ignore bytes (14-15) all zero.
func TestExternalFrameMatchesSpec(t *testing.T) {
	frame := sysex.Build(sysex.Cmd1External, sysex.ExtControllerFunc, [4]byte{0, 0, 0, 0}, nil)

	want := []byte{0xF0, 0x00, 0x21, 0x24, 0x08, 0x00, 0x70, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF7}
	want[len(want)-2] = sysex.Checksum(want)

	if !bytes.Equal(frame, want) {
		t.Errorf("bank-up frame:\ngot  % X\nwant % X", frame, want)
	}
}

// TestPadExtName verifies names are space-padded to the MC8 Pro's
// 32-byte field and over-long names are rejected.
func TestPadExtName(t *testing.T) {
	got, err := padExtName("HELLO")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 {
		t.Fatalf("padded length: got %d, want 32", len(got))
	}
	if string(got[:5]) != "HELLO" || got[5] != ' ' || got[31] != ' ' {
		t.Errorf("padding wrong: % X", got)
	}
	if _, err := padExtName("123456789012345678901234567890123"); err == nil {
		t.Error("expected error for 33-char name")
	}
}

// TestSetPresetMessageValidation verifies range and type checks fire
// before any I/O (so they can be tested without a device).
func TestSetPresetMessageValidation(t *testing.T) {
	c := &Client{log: discardLogger()}
	ctx := context.Background()

	if err := c.SetPresetMessage(ctx, 24, 0, Message{Type: model.MsgTypeCC}, false); err == nil {
		t.Error("expected error for preset 24")
	}
	if err := c.SetPresetMessage(ctx, 0, 16, Message{Type: model.MsgTypeCC}, false); err == nil {
		t.Error("expected error for slot 16 (external API is 0-15)")
	}
	if err := c.SetPresetMessage(ctx, 0, 0, Message{Type: model.MsgTypeNoteOn}, false); err == nil {
		t.Error("expected error for non-PC/CC type")
	}
}
