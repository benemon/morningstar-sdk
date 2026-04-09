package sysex

import (
	"bytes"
	"testing"
)

// TestChecksumKnownFrames verifies the XOR-and-mask-7-bits checksum
// against frames we captured during protocol reverse engineering.
// These are reference values copied verbatim from Protokol logs and
// the editor.js source.
func TestChecksumKnownFrames(t *testing.T) {
	cases := []struct {
		name  string
		frame []byte
		want  byte
	}{
		{
			// Outgoing ping we built in main.go and confirmed the
			// device replied to.
			name: "outgoing ping (byte 5 = 0)",
			frame: []byte{
				0xF0, 0x00, 0x21, 0x24, 0x08, 0x00, 0x00, 0x7D,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0xF7,
			},
			want: 0x00,
		},
		{
			// Device ping reply (session active) — observed in
			// Protokol. Byte 5 = 0x04.
			name: "incoming ping reply session active",
			frame: []byte{
				0xF0, 0x00, 0x21, 0x24, 0x08, 0x04, 0x00, 0x7D,
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12,
				0x17, 0xF7,
			},
			want: 0x17,
		},
		{
			// Device ping reply (no session) — observed in Protokol
			// when we pinged without a prior session-open.
			name: "incoming ping reply no session",
			frame: []byte{
				0xF0, 0x00, 0x21, 0x24, 0x08, 0x04, 0x00, 0x7D,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12,
				0x16, 0xF7,
			},
			want: 0x16,
		},
		{
			// Session-open editor→device (cmd 00 1B). This is the
			// frame our Go probe built and sent that put the MC8 Pro
			// into edit mode.
			name: "outgoing session open",
			frame: []byte{
				0xF0, 0x00, 0x21, 0x24, 0x08, 0x00, 0x00, 0x1B,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x66, 0xF7,
			},
			want: 0x66,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Checksum(tc.frame); got != tc.want {
				t.Errorf("Checksum = 0x%02X, want 0x%02X", got, tc.want)
			}
		})
	}
}

// TestBuildOutgoing confirms that Build produces byte-identical frames
// to the outgoing ping and session-open our probe sent and the device
// accepted. If this passes, Build is a drop-in replacement for the
// hand-rolled frame construction in main.go.
func TestBuildOutgoing(t *testing.T) {
	cases := []struct {
		name string
		cmd1 byte
		cmd2 byte
		want []byte
	}{
		{
			// Verified against the ping we sent from main.go that the
			// device replied to with session state.
			name: "ping",
			cmd1: Cmd1General,
			cmd2: CmdPing,
			want: []byte{
				0xF0, 0x00, 0x21, 0x24, 0x08, 0x00, 0x00, 0x7D,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0xF7,
			},
		},
		{
			// Verified against the session-open frame that actually
			// put the MC8 Pro into edit mode.
			name: "session open",
			cmd1: Cmd1General,
			cmd2: CmdSessionOpen,
			want: []byte{
				0xF0, 0x00, 0x21, 0x24, 0x08, 0x00, 0x00, 0x1B,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x66, 0xF7,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Build(tc.cmd1, tc.cmd2, NoArgs, nil)
			if !bytes.Equal(got, tc.want) {
				t.Errorf("Build produced wrong bytes\n got: % X\nwant: % X", got, tc.want)
			}
		})
	}
}

// TestParseKnownFrames decodes captured frames and verifies the
// header fields come out as expected.
func TestParseKnownFrames(t *testing.T) {
	cases := []struct {
		name    string
		frame   []byte
		wantVer byte
		wantC1  byte
		wantC2  byte
		wantArg [4]byte
	}{
		{
			name: "incoming ping reply session active",
			frame: []byte{
				0xF0, 0x00, 0x21, 0x24, 0x08, 0x04, 0x00, 0x7D,
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12,
				0x17, 0xF7,
			},
			wantVer: VersionIncoming,
			wantC1:  0x00,
			wantC2:  CmdPing,
			wantArg: [4]byte{0x01, 0x00, 0x00, 0x00},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.frame)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if f.Model != ModelMC8Pro {
				t.Errorf("Model = %d, want %d", f.Model, ModelMC8Pro)
			}
			if f.Version != tc.wantVer {
				t.Errorf("Version = %d, want %d", f.Version, tc.wantVer)
			}
			if f.Cmd1 != tc.wantC1 || f.Cmd2 != tc.wantC2 {
				t.Errorf("Cmd = %02X %02X, want %02X %02X", f.Cmd1, f.Cmd2, tc.wantC1, tc.wantC2)
			}
			if f.Args != tc.wantArg {
				t.Errorf("Args = %v, want %v", f.Args, tc.wantArg)
			}
		})
	}
}

// TestParseRejectsInvalid exercises the error paths in Parse.
func TestParseRejectsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		frame []byte
		want  error
	}{
		{"too short", []byte{0xF0, 0xF7}, ErrTooShort},
		{"no start", []byte{
			0x00, 0x00, 0x21, 0x24, 0x08, 0x00, 0x00, 0x7D,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0xF7,
		}, ErrBadStart},
		{"no end", []byte{
			0xF0, 0x00, 0x21, 0x24, 0x08, 0x00, 0x00, 0x7D,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00,
		}, ErrBadEnd},
		{"wrong manufacturer", []byte{
			0xF0, 0x41, 0x00, 0x00, 0x08, 0x00, 0x00, 0x7D,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0xF7,
		}, ErrBadManufacturer},
		{"bad checksum", []byte{
			0xF0, 0x00, 0x21, 0x24, 0x08, 0x00, 0x00, 0x7D,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x7F, 0xF7, // wrong checksum
		}, ErrBadChecksum},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.frame); err != tc.want {
				t.Errorf("Parse error = %v, want %v", err, tc.want)
			}
		})
	}
}
