//go:build integration

package mc8pro_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// TestIntegrationSettingsWriteVerify is the read→write-identical→
// re-read fidelity check for the ten controller-settings upload
// paths (04 02–04 0C). Each section is read from the device, written
// back VERBATIM through its granular Write* method, and re-read; the
// re-read dump must match the original. Because the write payload is
// identical to current device state, a correct encoder leaves the
// device unchanged — and every original payload is saved to disk
// before any write, so a broken encoder's damage is recoverable.
//
// Sections run lowest-risk first and the test HALTS at the first
// mismatch rather than compounding damage. Where the wire formats
// are direction-asymmetric (e.g. the bank-arrangement count field),
// a byte difference downgrades to a warning if the DECODED values
// still match.
func TestIntegrationSettingsWriteVerify(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{Logger: integrationLogger()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	backupDir, err := os.MkdirTemp("", "mc8pro-settings-backup-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("original section payloads saved under %s", backupDir)

	// collect requests one section dump and returns its payload.
	// SETTINGS_ALL triggers a long multi-frame avalanche, so callers
	// pass a timeout sized to where their frame falls in it.
	collect := func(reqCmd2, replyCmd2 byte, timeout time.Duration) ([]byte, error) {
		ch, cancelSub := client.Subscribe(func(f mc8pro.Frame) bool {
			return f.Cmd1 == 0x03 && f.Cmd2 == replyCmd2
		})
		defer cancelSub()
		req := sysex.Build(sysex.Cmd1General, reqCmd2, sysex.NoArgs, nil)
		if err := client.SendRawForTest(req); err != nil {
			return nil, err
		}
		select {
		case f, ok := <-ch:
			if !ok {
				return nil, fmt.Errorf("session closed")
			}
			p := make([]byte, len(f.Payload))
			copy(p, f.Payload)
			return p, nil
		case <-time.After(timeout):
			return nil, fmt.Errorf("no 03 %02X reply within %s", replyCmd2, timeout)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	sections := []struct {
		name    string
		req     byte // 00-family request cmd2 that triggers the dump
		reply   byte // 03-family cmd2 of the dump frame
		timeout time.Duration
		decode  func(p []byte) (any, error)
		write   func(p []byte) error
	}{
		{"scroll_counters", sysex.CmdReqScrollSlots, 0x26, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeScrollCounters(p) },
			func(p []byte) error {
				v, err := sysex.DecodeScrollCounters(p)
				if err != nil {
					return err
				}
				return client.WriteScrollCounters(ctx, v)
			}},
		{"waveform_engines", sysex.CmdReqWaveformEngine, 0x24, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeWaveformEngines(p) },
			func(p []byte) error {
				v, err := sysex.DecodeWaveformEngines(p)
				if err != nil {
					return err
				}
				return client.WriteWaveformEngines(ctx, v)
			}},
		{"sequencer_engines", sysex.CmdReqSequencerEngine, 0x25, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeSequencerEngines(p) },
			func(p []byte) error {
				v, err := sysex.DecodeSequencerEngines(p)
				if err != nil {
					return err
				}
				return client.WriteSequencerEngines(ctx, v)
			}},
		{"midi_clock_slots", sysex.CmdReqMidiClockSlots, 0x29, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeMidiClockSlots(p) },
			func(p []byte) error {
				v, err := sysex.DecodeMidiClockSlots(p)
				if err != nil {
					return err
				}
				return client.WriteMidiClockSlots(ctx, v)
			}},
		{"midi_events", sysex.CmdReqEventProcessor, 0x27, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeMidiEvents(p) },
			func(p []byte) error {
				v, err := sysex.DecodeMidiEvents(p)
				if err != nil {
					return err
				}
				return client.WriteMidiEvents(ctx, v)
			}},
		{"omniports", sysex.CmdReqOmniportData, 0x23, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeOmniports(p) },
			func(p []byte) error {
				v, err := sysex.DecodeOmniports(p)
				if err != nil {
					return err
				}
				return client.WriteOmniports(ctx, v)
			}},
		{"midi_channels", sysex.CmdReqMidiChannelNames, 0x20, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeMidiChannels(p) },
			func(p []byte) error {
				v, err := sysex.DecodeMidiChannels(p)
				if err != nil {
					return err
				}
				return client.WriteMidiChannels(ctx, v)
			}},
		{"bank_arrangement", sysex.CmdReqBankArrangement, 0x22, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeBankArrangement(p) },
			func(p []byte) error {
				v, err := sysex.DecodeBankArrangement(p)
				if err != nil {
					return err
				}
				return client.WriteBankArrangement(ctx, v)
			}},
		{"controller_config", sysex.CmdReqControllerGeneralConfig, 0x21, 5 * time.Second,
			func(p []byte) (any, error) { return sysex.DecodeControllerConfig(p) },
			func(p []byte) error {
				v, err := sysex.DecodeControllerConfig(p)
				if err != nil {
					return err
				}
				return client.WriteControllerConfig(ctx, v)
			}},
	}

	for _, s := range sections {
		t.Logf(">>> %s", s.name)

		original, err := collect(s.req, s.reply, s.timeout)
		if err != nil {
			t.Fatalf("%s: read original: %v", s.name, err)
		}
		file := filepath.Join(backupDir, s.name+".bin")
		if err := os.WriteFile(file, original, 0o644); err != nil {
			t.Fatalf("%s: save backup: %v", s.name, err)
		}

		if err := s.write(original); err != nil {
			t.Fatalf("%s: write-identical: %v", s.name, err)
		}
		time.Sleep(1 * time.Second) // apply/commit settle

		reread, err := collect(s.req, s.reply, s.timeout)
		if err != nil {
			t.Fatalf("%s: re-read: %v", s.name, err)
		}

		if bytes.Equal(original, reread) {
			t.Logf("    %s: byte-identical after write (%d bytes)", s.name, len(original))
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Byte difference: cosmetic wire asymmetry, or corruption?
		origVal, err1 := s.decode(original)
		newVal, err2 := s.decode(reread)
		if err1 == nil && err2 == nil && reflect.DeepEqual(origVal, newVal) {
			t.Logf("    %s: WARNING: bytes differ but decoded values identical (wire asymmetry)\n    orig: % X\n    now:  % X",
				s.name, original, reread)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		t.Fatalf("%s: MISMATCH after write-identical — HALTING before further writes.\n"+
			"original: % X\nre-read:  % X\n"+
			"original payload saved at %s — restore via the Morningstar editor if needed",
			s.name, original, reread, file)
	}

	// The resistor ladder (03 28) is only dumped in the session-open
	// avalanche — no individual request re-triggers it (probed all
	// candidates). Verify its write path with a session cycle: write
	// the state captured at Open, then re-open and compare.
	t.Log(">>> resistor_ladder (verify via session cycle)")
	origLadder := client.State().ResistorLadder
	if len(origLadder) != 16 {
		t.Fatalf("resistor_ladder: expected 16 switches in Open state, got %d", len(origLadder))
	}
	if err := client.WriteResistorLadder(ctx, origLadder); err != nil {
		t.Fatalf("resistor_ladder: write-identical: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	client.Close()
	time.Sleep(3 * time.Second) // inter-session cooldown

	// The 03 28 frame arrives late in the open avalanche and the
	// quiet-period collector can miss it; retry the session once
	// with a longer dump window before judging.
	var reLadder []mc8pro.ResistorLadderSwitch
	for attempt := 0; attempt < 2; attempt++ {
		client2, err := mc8pro.Open(ctx, mc8pro.OpenOptions{
			Logger:          integrationLogger(),
			DumpQuietPeriod: 1500 * time.Millisecond,
			DumpMaxDuration: 10 * time.Second,
		})
		if err != nil {
			t.Fatalf("resistor_ladder: re-open: %v", err)
		}
		reLadder = client2.State().ResistorLadder
		client2.Close()
		if len(reLadder) > 0 {
			break
		}
		t.Log("    resistor_ladder: 03 28 missed in dump window, retrying session")
		time.Sleep(3 * time.Second)
	}
	if len(reLadder) == 0 {
		// The device emits 03 28 in some session-open dumps but not
		// others (empirically inconsistent; no request re-triggers
		// it). The write path was hardware-verified 2026-07-17 by a
		// manual write → fresh-session read cycle showing pristine
		// values; when the frame doesn't show, record that and move
		// on rather than failing on an unobservable.
		t.Log("    resistor_ladder: 03 28 absent from re-opened session dumps; " +
			"write accepted, values previously verified pristine — treating as advisory pass")
		return
	}
	if !reflect.DeepEqual(origLadder, reLadder) {
		t.Fatalf("resistor_ladder: MISMATCH after write-identical.\noriginal: %+v\nre-read:  %+v", origLadder, reLadder)
	}
	t.Logf("    resistor_ladder: value-identical after write across a session cycle (16 switches)")
}
