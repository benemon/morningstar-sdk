package mc8pro

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// This file implements the OFFICIAL external-application SysEx API
// (command family 0x70), documented in
// reference/sysex-api-external-applications.pdf. Unlike the editor
// session protocol, this API is documented and firmware-validated:
// the device checks the frame checksum and payload sizes and replies
// with a typed error frame (op2 0x7F) on rejection.
//
// Write methods take a `save` flag: true commits to flash; false
// applies a TEMPORARY override that reverts when the device changes
// bank. Temporary writes are ideal for live use and for testing
// without flash wear.

// extNameLen is the MC8 Pro's name field size for the external API
// (short, toggle, long, and bank names are all 32 on Pro models; the
// API requires names padded to exactly this length).
const extNameLen = 32

// extErrorWindow is how long write methods wait for a device error
// reply before assuming success. The device replies with an ExtError
// frame only on failure, so success is inferred from silence.
const extErrorWindow = 250 * time.Millisecond

// ControllerInfo is the reply to [Client.GetControllerInfo] (external
// API op2 0x32).
type ControllerInfo struct {
	Model             int    // device model ID (8 = MC8 Pro)
	Firmware          [4]int // firmware version bytes as sent by the device
	MessagesPerPreset int
	ShortNameLen      int
	LongNameLen       int
	BankNameLen       int
}

// PresetOptions selects which preset flags and colors an
// [Client.SetPresetOptions] call updates. Nil fields are left
// unchanged on the device. Color values use the palette indices from
// the API's color chart (0–63 solid, dim variants above).
type PresetOptions struct {
	ToToggle    *bool
	ToBlink     *bool
	ToMsgScroll *bool
	ToggleGroup *int // 0 = independent, 1–16 = groups

	Pos1LedColor, Pos2LedColor, ShiftLedColor                      *int
	Pos1TextColor, Pos2TextColor, ShiftTextColor                   *int
	Pos1BackgroundColor, Pos2BackgroundColor, ShiftBackgroundColor *int
}

// extAckError describes an error reply from the device.
func extAckError(code byte) error {
	msg := "unknown error"
	switch code {
	case sysex.ExtAckWrongModelID:
		msg = "wrong model ID"
	case sysex.ExtAckWrongChecksum:
		msg = "wrong checksum"
	case sysex.ExtAckWrongPayloadSize:
		msg = "wrong payload size"
	}
	return fmt.Errorf("mc8pro: device rejected external command: %s (ack 0x%02X)", msg, code)
}

// extSend sends one external-API frame and waits up to
// extErrorWindow for the device's ack. Hardware-observed behavior
// (contrary to the doc's errors-only description): data writes are
// acked on SUCCESS as well (op2 0x7F, ack 0), while controller
// functions (op2 0x00 bank up/down, toggle page) are not acked at
// all — hence the timeout-means-success fallback.
//
// NOTE: the ack means the frame was ACCEPTED, not yet APPLIED — a
// read-back issued immediately after a write can return the old
// value. Allow a brief settle (~300ms observed sufficient on fw
// 3.13.6) between a write and its verifying read.
func (c *Client) extSend(ctx context.Context, op2 byte, args [4]byte, payload []byte) error {
	ch, cancel := c.router.subscribe(func(f sysex.Frame) bool {
		return f.Cmd1 == sysex.Cmd1External && f.Cmd2 == sysex.ExtError
	})
	defer cancel()

	frame := sysex.Build(sysex.Cmd1External, op2, args, payload)
	if err := c.port.Send(frame); err != nil {
		return fmt.Errorf("mc8pro: send external cmd 0x%02X: %w", op2, err)
	}

	select {
	case f, ok := <-ch:
		if !ok {
			return fmt.Errorf("mc8pro: session closed")
		}
		if f.Args[0] == sysex.ExtAckSuccess {
			return nil
		}
		return extAckError(f.Args[0])
	case <-time.After(extErrorWindow):
		return nil // no error reply = accepted
	case <-ctx.Done():
		return ctx.Err()
	}
}

// extRequest sends one external-API request and waits for the reply
// frame, which echoes the request op2 (hardware-verified for every
// Get function; the doc's claim that Get Current Bank Name replies
// with op2 0x21 is a doc error — the device echoes 0x30). Frames
// with any other op2 are skipped: SUCCESS acks (op2 0x7F, ack 0)
// arrive for preceding writes too — the device acks successful
// writes, contrary to the doc's errors-only description — and
// delayed replies to earlier requests must not be mistaken for this
// one's. Error replies (op2 0x7F, non-zero ack) surface as errors.
func (c *Client) extRequest(ctx context.Context, op2 byte, args [4]byte) (sysex.Frame, error) {
	ch, cancel := c.router.subscribe(func(f sysex.Frame) bool {
		return f.Cmd1 == sysex.Cmd1External
	})
	defer cancel()

	frame := sysex.Build(sysex.Cmd1External, op2, args, nil)
	if err := c.port.Send(frame); err != nil {
		return sysex.Frame{}, fmt.Errorf("mc8pro: send external cmd 0x%02X: %w", op2, err)
	}

	deadline := time.NewTimer(c.opts.FrameTimeout)
	defer deadline.Stop()
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				return sysex.Frame{}, fmt.Errorf("mc8pro: session closed")
			}
			if f.Cmd2 == sysex.ExtError {
				if f.Args[0] != sysex.ExtAckSuccess {
					return sysex.Frame{}, extAckError(f.Args[0])
				}
				continue // stale success ack from a preceding write
			}
			if f.Cmd2 != op2 {
				continue // delayed reply to an earlier request
			}
			return f, nil
		case <-deadline.C:
			return sysex.Frame{}, fmt.Errorf("mc8pro: external cmd 0x%02X: no reply within %s", op2, c.opts.FrameTimeout)
		case <-ctx.Done():
			return sysex.Frame{}, ctx.Err()
		}
	}
}

// padExtName space-pads a name to the model's fixed field size, as
// the external API requires.
func padExtName(name string) ([]byte, error) {
	if len(name) > extNameLen {
		return nil, fmt.Errorf("mc8pro: name %q is %d chars, max %d", name, len(name), extNameLen)
	}
	out := make([]byte, extNameLen)
	for i := range out {
		out[i] = ' '
	}
	copy(out, name)
	return out, nil
}

// saveByte converts a save flag to the API's op argument: ExtSave
// (0x7F) commits to flash, 0 = temporary override.
func saveByte(save bool) byte {
	if save {
		return sysex.ExtSave
	}
	return 0
}

// BankUp moves the device to the next bank (external API, documented
// equivalent of the front-panel C+D press).
func (c *Client) BankUp(ctx context.Context) error {
	c.log.Info("external bank up")
	return c.extSend(ctx, sysex.ExtControllerFunc, [4]byte{0, 0, 0, 0}, nil)
}

// BankDown moves the device to the previous bank.
func (c *Client) BankDown(ctx context.Context) error {
	c.log.Info("external bank down")
	return c.extSend(ctx, sysex.ExtControllerFunc, [4]byte{1, 0, 0, 0}, nil)
}

// TogglePage flips the device to its next preset page.
func (c *Client) TogglePage(ctx context.Context) error {
	c.log.Info("external toggle page")
	return c.extSend(ctx, sysex.ExtControllerFunc, [4]byte{2, 0, 0, 0}, nil)
}

// setPresetName implements the three name-update functions, which
// differ only in op2.
func (c *Client) setPresetName(ctx context.Context, op2 byte, preset int, name string, save bool) error {
	if preset < 0 || preset > 23 {
		return fmt.Errorf("mc8pro: preset index %d out of range 0..23", preset)
	}
	payload, err := padExtName(name)
	if err != nil {
		return err
	}
	c.log.Info("external set preset name",
		slog.Int("op2", int(op2)), slog.Int("preset", preset),
		slog.String("name", name), slog.Bool("save", save))
	return c.extSend(ctx, op2, [4]byte{byte(preset), saveByte(save), 0, 0}, payload)
}

// SetPresetShortName updates a preset's short name (the switch
// label) in the current bank. save=false applies a temporary
// override that reverts on bank change.
func (c *Client) SetPresetShortName(ctx context.Context, preset int, name string, save bool) error {
	return c.setPresetName(ctx, sysex.ExtSetPresetShort, preset, name, save)
}

// SetPresetToggleName updates a preset's toggle (Position 2) name in
// the current bank.
func (c *Client) SetPresetToggleName(ctx context.Context, preset int, name string, save bool) error {
	return c.setPresetName(ctx, sysex.ExtSetPresetToggle, preset, name, save)
}

// SetPresetLongName updates a preset's long name in the current bank.
func (c *Client) SetPresetLongName(ctx context.Context, preset int, name string, save bool) error {
	return c.setPresetName(ctx, sysex.ExtSetPresetLong, preset, name, save)
}

// SetPresetMessage updates one message slot of a preset in the
// current bank via the external API. Only slots 0–15 and message
// types PC and CC are supported by this API (the editor session
// protocol's WritePreset covers all 32 slots and every type). The
// message's Action, Toggle (position), Data and Channel fields are
// taken from m; other fields are ignored.
func (c *Client) SetPresetMessage(ctx context.Context, preset, slot int, m Message, save bool) error {
	if preset < 0 || preset > 23 {
		return fmt.Errorf("mc8pro: preset index %d out of range 0..23", preset)
	}
	if slot < 0 || slot > 15 {
		return fmt.Errorf("mc8pro: external API message slot %d out of range 0..15", slot)
	}
	var payload []byte
	switch m.Type {
	case model.MsgTypePC:
		payload = []byte{byte(m.Action), byte(m.Toggle), byte(m.Data[0]), byte(m.Channel)}
	case model.MsgTypeCC:
		payload = []byte{byte(m.Action), byte(m.Toggle), byte(m.Data[0]), byte(m.Data[1]), byte(m.Channel)}
	default:
		return fmt.Errorf("mc8pro: external API supports PC and CC messages only, got type %d", m.Type)
	}
	c.log.Info("external set preset message",
		slog.Int("preset", preset), slog.Int("slot", slot),
		slog.Int("type", m.Type), slog.Bool("save", save))
	args := [4]byte{byte(preset), byte(slot), byte(m.Type), saveByte(save)}
	return c.extSend(ctx, sysex.ExtSetPresetMessage, args, payload)
}

// SetPresetOptions updates a preset's toggle/blink/scroll flags,
// toggle group, and colors in the current bank. Nil fields in opts
// are left unchanged.
//
// LIMITATION (hardware-verified on firmware 3.13.6): the save flag
// is ineffective for this function — probes at every plausible
// opcode position (op4–op7) failed to persist across a bank change,
// so op2 05 writes always behave as TEMPORARY overrides regardless
// of save. To persist preset options, use [Client.WritePreset]. The
// flag is still sent at the documented position in case a future
// firmware honors it.
//
// COLOR QUIRK (hardware-verified): color values pass through a
// firmware translation layer and do NOT map 1:1 to the stored
// palette indices — writing 9 (the API chart's RED) stored palette
// index 51 on fw 3.13.6, matching forum reports that the translation
// follows the editor UI's picker order and "breaks down" for higher
// values. Colors set here will not read back as the same index via
// ReadBank; for exact palette indices use [Client.WritePreset].
func (c *Client) SetPresetOptions(ctx context.Context, preset int, opts PresetOptions, save bool) error {
	if preset < 0 || preset > 23 {
		return fmt.Errorf("mc8pro: preset index %d out of range 0..23", preset)
	}
	// Tri-state bool: 0x7F = on, 0x00 = off, anything else = keep.
	// 0x01 is the documented "other" we use for keep.
	triBool := func(v *bool) byte {
		switch {
		case v == nil:
			return 0x01
		case *v:
			return 0x7F
		default:
			return 0x00
		}
	}
	group := byte(0x7F) // out-of-range = keep
	if opts.ToggleGroup != nil {
		if *opts.ToggleGroup < 0 || *opts.ToggleGroup > 16 {
			return fmt.Errorf("mc8pro: toggle group %d out of range 0..16", *opts.ToggleGroup)
		}
		group = byte(*opts.ToggleGroup)
	}
	color := func(v *int) byte {
		if v == nil {
			return sysex.ExtColorKeep
		}
		return byte(*v & 0x7F)
	}
	payload := []byte{
		triBool(opts.ToToggle),
		triBool(opts.ToBlink),
		triBool(opts.ToMsgScroll),
		group,
		color(opts.Pos1LedColor), color(opts.Pos2LedColor), color(opts.ShiftLedColor),
		color(opts.Pos1TextColor), color(opts.Pos2TextColor), color(opts.ShiftTextColor),
		color(opts.Pos1BackgroundColor), color(opts.Pos2BackgroundColor), color(opts.ShiftBackgroundColor),
	}
	c.log.Info("external set preset options",
		slog.Int("preset", preset), slog.Bool("save", save))
	args := [4]byte{byte(preset), 0, 0, saveByte(save)}
	return c.extSend(ctx, sysex.ExtSetPresetOther, args, payload)
}

// SetBankName updates the CURRENT bank's name. save=false applies a
// temporary override that reverts on bank change.
func (c *Client) SetBankName(ctx context.Context, name string, save bool) error {
	payload, err := padExtName(name)
	if err != nil {
		return err
	}
	c.log.Info("external set bank name", slog.String("name", name), slog.Bool("save", save))
	return c.extSend(ctx, sysex.ExtSetBankName, [4]byte{0, saveByte(save), 0, 0}, payload)
}

// DisplayMessage shows a message of up to 20 characters on the
// device's LCD for the given duration (rounded down to 100ms
// increments, max 12.7s).
func (c *Client) DisplayMessage(ctx context.Context, text string, duration time.Duration) error {
	if len(text) > 20 {
		return fmt.Errorf("mc8pro: display message %q is %d chars, max 20", text, len(text))
	}
	ticks := int(duration / (100 * time.Millisecond))
	if ticks < 0 {
		ticks = 0
	}
	if ticks > 127 {
		ticks = 127
	}
	c.log.Info("external display message", slog.String("text", text), slog.Int("ticks", ticks))
	return c.extSend(ctx, sysex.ExtDisplayMessage, [4]byte{0, byte(ticks), 0, 0}, []byte(text))
}

// getName implements the name-read functions.
func (c *Client) getName(ctx context.Context, op2 byte, preset int) (string, error) {
	f, err := c.extRequest(ctx, op2, [4]byte{byte(preset), 0, 0, 0})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(f.Payload), " "), nil
}

// GetPresetShortName reads a preset's short name from the current
// bank via the external API (reflects any temporary override).
func (c *Client) GetPresetShortName(ctx context.Context, preset int) (string, error) {
	if preset < 0 || preset > 23 {
		return "", fmt.Errorf("mc8pro: preset index %d out of range 0..23", preset)
	}
	return c.getName(ctx, sysex.ExtGetPresetShort, preset)
}

// GetPresetToggleName reads a preset's toggle name from the current bank.
func (c *Client) GetPresetToggleName(ctx context.Context, preset int) (string, error) {
	if preset < 0 || preset > 23 {
		return "", fmt.Errorf("mc8pro: preset index %d out of range 0..23", preset)
	}
	return c.getName(ctx, sysex.ExtGetPresetToggle, preset)
}

// GetPresetLongName reads a preset's long name from the current bank.
func (c *Client) GetPresetLongName(ctx context.Context, preset int) (string, error) {
	if preset < 0 || preset > 23 {
		return "", fmt.Errorf("mc8pro: preset index %d out of range 0..23", preset)
	}
	return c.getName(ctx, sysex.ExtGetPresetLong, preset)
}

// GetBankName reads the current bank's name via the external API.
func (c *Client) GetBankName(ctx context.Context) (string, error) {
	return c.getName(ctx, sysex.ExtGetBankName, 0)
}

// GetPresetToggles reads the toggle state of every preset in the
// current bank. Index i is true when preset i is in Position 2.
func (c *Client) GetPresetToggles(ctx context.Context) ([]bool, error) {
	f, err := c.extRequest(ctx, sysex.ExtGetToggleStates, [4]byte{})
	if err != nil {
		return nil, err
	}
	n := int(f.Args[1]) // op4 = total number of presets
	if n <= 0 || n > len(f.Payload) {
		n = len(f.Payload)
	}
	states := make([]bool, n)
	for i := 0; i < n; i++ {
		states[i] = f.Payload[i] == 0x7F
	}
	return states, nil
}

// GetControllerInfo reads the device's model, firmware version, and
// capacity limits via the external API.
func (c *Client) GetControllerInfo(ctx context.Context) (ControllerInfo, error) {
	f, err := c.extRequest(ctx, sysex.ExtGetControllerInfo, [4]byte{})
	if err != nil {
		return ControllerInfo{}, err
	}
	if len(f.Payload) < 9 {
		return ControllerInfo{}, fmt.Errorf("mc8pro: controller info payload has %d bytes, want 9", len(f.Payload))
	}
	p := f.Payload
	return ControllerInfo{
		Model:             int(p[0]),
		Firmware:          [4]int{int(p[1]), int(p[2]), int(p[3]), int(p[4])},
		MessagesPerPreset: int(p[5]),
		ShortNameLen:      int(p[6]),
		LongNameLen:       int(p[7]),
		BankNameLen:       int(p[8]),
	}, nil
}
