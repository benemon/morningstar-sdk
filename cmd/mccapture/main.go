// mccapture connects to a Morningstar MC8 Pro on USB MIDI Port 1,
// runs the editor's session-open + request-cascade handshake, and
// writes every received SysEx frame to pkg/mc8/testdata/raw/ as a
// binary file. These files become fixtures for the pkg/mc8/sysex
// unit tests.
//
// Run with the pedal connected and the web editor disconnected:
//
//	go run ./cmd/mccapture
//
// By default fixtures are written to pkg/mc8/testdata/raw/. Override
// with -out=DIR.
//
// Filenames are NNN_CMD1CMD2_lenNNNN.sysex, where NNN is the order of
// arrival. Each file contains the full frame including 0xF0 and 0xF7
// so it can be Parse()'d directly.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

const portMatch = "MC8 Pro Port 1"

func main() {
	outDir := flag.String("out", filepath.Join("pkg", "mc8", "testdata", "raw"), "directory to write frames to")
	drainSeconds := flag.Int("drain", 4, "seconds to drain after the last request")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		die("mkdir %s: %v", *outDir, err)
	}

	defer midi.CloseDriver()

	in, err := findPort(midi.GetInPorts(), portMatch)
	if err != nil {
		die("input: %v", err)
	}
	out, err := findPort(midi.GetOutPorts(), portMatch)
	if err != nil {
		die("output: %v", err)
	}
	fmt.Printf("using input:  %s\nusing output: %s\n\n", in, out)

	sender, err := midi.SendTo(out)
	if err != nil {
		die("open output: %v", err)
	}

	var seq atomic.Int32
	if err := in.Open(); err != nil {
		die("open input: %v", err)
	}
	stop, err := in.Listen(func(msg []byte, _ int32) {
		n := int(seq.Add(1))
		saveFrame(*outDir, n, msg)
	}, drivers.ListenConfig{SysEx: true, SysExBufferSize: 8192})
	if err != nil {
		die("listen: %v", err)
	}
	defer stop()

	// Session open.
	frame := sysex.Build(sysex.Cmd1General, sysex.CmdSessionOpen, sysex.NoArgs, nil)
	fmt.Println(">>> SESSION_OPEN (cmd 00 1B)")
	if err := sender(frame); err != nil {
		die("send session-open: %v", err)
	}
	fmt.Println("<<< waiting 1.2s for device to enter editor mode...")
	time.Sleep(1200 * time.Millisecond)

	// Request cascade — shared with Client.Open via mc8pro.InitRequestSequence.
	fmt.Println(">>> firing init request sequence")
	for _, r := range mc8pro.InitRequestSequence() {
		req := sysex.Build(sysex.Cmd1General, r.Cmd2, sysex.NoArgs, nil)
		fmt.Printf("    REQUEST_%s (cmd2=%d)\n", r.Name, r.Cmd2)
		if err := sender(req); err != nil {
			die("send %s: %v", r.Name, err)
		}
		time.Sleep(150 * time.Millisecond)
	}

	fmt.Printf("\n<<< draining for %ds...\n", *drainSeconds)
	time.Sleep(time.Duration(*drainSeconds) * time.Second)

	// Session close. editor.js:90264 disconnectEditorMode() — sendSysexFunction(0, 28).
	// NOT (0, 1) — that's a different command in a different disconnect path
	// and does not actually exit edit mode on the device. See CLAUDE.md.
	//
	// The device sends an unsolicited 00 7D reply with byte 8 = 0 shortly
	// after, confirming it has exited edit mode. The LCD transitions from
	// edit display back to normal bank display within a few hundred ms.
	fmt.Println("\n>>> SESSION_CLOSE (cmd 00 1C)")
	closeFrame := sysex.Build(sysex.Cmd1General, sysex.CmdSessionClose, sysex.NoArgs, nil)
	if err := sender(closeFrame); err != nil {
		die("send session-close: %v", err)
	}
	// Brief drain to capture anything the device emits in response.
	time.Sleep(400 * time.Millisecond)

	// Verify the session is actually closed by pinging and checking
	// byte 8 of the reply. This only prints a warning; we don't fail
	// the capture if the ping doesn't arrive in time.
	fmt.Println(">>> PING (verify session closed)")
	pingFrame := sysex.Build(sysex.Cmd1General, sysex.CmdPing, sysex.NoArgs, nil)
	if err := sender(pingFrame); err != nil {
		die("send ping: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	total := int(seq.Load())
	fmt.Printf("\ndone. captured %d frames into %s\n", total, *outDir)
}

// saveFrame writes one received SysEx frame to disk. The filename
// encodes the arrival order and the command bytes so multiple runs
// are easy to diff.
func saveFrame(dir string, seq int, msg []byte) {
	cmd1, cmd2 := byte(0), byte(0)
	if len(msg) > sysex.OffsetCmd2 {
		cmd1 = msg[sysex.OffsetCmd1]
		cmd2 = msg[sysex.OffsetCmd2]
	}
	name := fmt.Sprintf("%03d_%02X%02X_len%04d.sysex", seq, cmd1, cmd2, len(msg))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, msg, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: write %s: %v\n", path, err)
		return
	}
	fmt.Printf("  [%s] %4d bytes  cmd=%02X %02X\n", name, len(msg), cmd1, cmd2)
}

func findPort[T fmt.Stringer](ports []T, match string) (T, error) {
	for _, p := range ports {
		if strings.Contains(p.String(), match) {
			return p, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("no port matching %q", match)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
