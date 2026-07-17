package mc8pro

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // register driver
)

// midiPort is the live MIDI session for one Morningstar controller:
// an opened input port, an opened output port, and a background
// listener that delivers received frames via a callback.
//
// This is a private wrapper around gitlab.com/gomidi/midi/v2 so the
// rest of the package never has to know which driver is in use. If we
// ever need to swap drivers, this is the only file that changes.
type midiPort struct {
	in     drivers.In
	out    drivers.Out
	send   func([]byte) error
	stopRx func()
	log    *slog.Logger
	name   string
}

// midiSysExBufferSize is the per-frame buffer size handed to the
// driver's listener. Must be larger than the biggest SysEx frame the
// device might send. The 06 01 preset frame is 1032 bytes; we round
// up generously.
const midiSysExBufferSize = 8192

// openMIDIPort discovers a Morningstar input/output pair whose
// driver-supplied name contains portMatch (substring), opens both,
// and starts the receive callback running in the driver's goroutine.
// onFrame is called for every received SysEx frame.
//
// On any failure during opening, partially-opened resources are
// cleaned up before returning. The caller never has to "un-open" a
// half-open port.
func openMIDIPort(portMatch string, onFrame func([]byte), log *slog.Logger) (*midiPort, error) {
	if log == nil {
		log = discardLogger()
	}
	if portMatch == "" {
		return nil, errors.New("mc8pro: PortMatch is required")
	}

	in, err := findPort(midi.GetInPorts(), portMatch)
	if err != nil {
		return nil, fmt.Errorf("mc8pro: input port: %w", err)
	}
	out, err := findPort(midi.GetOutPorts(), portMatch)
	if err != nil {
		return nil, fmt.Errorf("mc8pro: output port: %w", err)
	}

	log = log.With(slog.String("component", "midi"), slog.String("port", in.String()))

	if err := in.Open(); err != nil {
		return nil, fmt.Errorf("mc8pro: open input: %w", err)
	}
	rawSend, err := midi.SendTo(out)
	if err != nil {
		_ = in.Close()
		return nil, fmt.Errorf("mc8pro: open output: %w", err)
	}
	// midi.SendTo returns func(midi.Message) error; midi.Message is a
	// []byte alias but Go won't implicitly convert function signatures.
	// Wrap to expose a clean []byte API.
	send := func(b []byte) error { return rawSend(b) }

	stopRx, err := in.Listen(func(msg []byte, _ int32) {
		log.Debug("frame received", frameAttrs(msg)...)
		onFrame(msg)
	}, drivers.ListenConfig{SysEx: true, SysExBufferSize: midiSysExBufferSize})
	if err != nil {
		_ = in.Close()
		return nil, fmt.Errorf("mc8pro: start listener: %w", err)
	}

	log.Info("MIDI port opened")

	return &midiPort{
		in:     in,
		out:    out,
		send:   send,
		stopRx: stopRx,
		log:    log,
		name:   in.String(),
	}, nil
}

// Send sends one complete SysEx frame to the device. The frame must
// already include 0xF0 ... 0xF7 framing and a valid checksum.
func (p *midiPort) Send(frame []byte) error {
	p.log.Debug("frame sent", frameAttrs(frame)...)
	return p.send(frame)
}

// Close stops the listener and closes the input port. Idempotent and
// safe to call from any goroutine. Returns the first error
// encountered, if any.
func (p *midiPort) Close() error {
	if p == nil {
		return nil
	}
	if p.stopRx != nil {
		p.stopRx()
		p.stopRx = nil
	}
	var err error
	if p.in != nil {
		if e := p.in.Close(); e != nil {
			err = fmt.Errorf("mc8pro: close input: %w", e)
		}
		p.in = nil
	}
	// gomidi's SendTo doesn't expose a "close output" — the output
	// port is reference-counted internally and will be released when
	// the driver is closed at process exit (midi.CloseDriver).
	p.log.Info("MIDI port closed")
	return err
}

// findPort returns the first port whose String() contains match.
func findPort[T fmt.Stringer](ports []T, match string) (T, error) {
	for _, p := range ports {
		if strings.Contains(p.String(), match) {
			return p, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("no port matching %q", match)
}

// DevicePort describes one detected Morningstar MC8 Pro MIDI port
// pair available on the system.
type DevicePort struct {
	Name string // driver-supplied port name (e.g. "Morningstar MC8 Pro Port 1")
}

// ListDevices enumerates all MIDI input ports whose name contains
// "MC8 Pro" and returns them as DevicePort entries. This does not
// open any ports or start a session — it's a pre-connection probe.
// Returns an empty slice (not an error) if no devices are found.
func ListDevices() []DevicePort {
	var devices []DevicePort
	for _, p := range midi.GetInPorts() {
		name := p.String()
		if strings.Contains(name, "MC8 Pro") {
			devices = append(devices, DevicePort{Name: name})
		}
	}
	return devices
}

// frameAttrs returns slog attributes describing a SysEx frame for
// debug logging: length plus cmd1/cmd2 if the frame is long enough.
// Returns a single []any so it spreads cleanly into log.Debug(...).
func frameAttrs(frame []byte) []any {
	attrs := []any{slog.Int("len", len(frame))}
	if len(frame) >= 8 {
		attrs = append(attrs,
			slog.String("cmd1", fmt.Sprintf("%02X", frame[6])),
			slog.String("cmd2", fmt.Sprintf("%02X", frame[7])),
		)
	}
	return attrs
}
