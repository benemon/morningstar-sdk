package mc8pro

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// Default values for OpenOptions. The settle time was determined
// empirically: the editor itself waits 1 second after sending the
// session-open before issuing requests (editor.js:90426-90430), and
// the device silently ignores requests during that window. We add a
// small margin. The dump timings were chosen based on observed
// inter-frame gaps (<10ms within a burst) and total dump duration
// (~1.6s for full connect, ~50ms for a bank switch).
const (
	defaultPortMatch       = "MC8 Pro Port 1"
	defaultSettleTime      = 1200 * time.Millisecond
	defaultPingTimeout     = 1 * time.Second
	defaultFrameTimeout    = 2 * time.Second
	defaultDumpQuietPeriod = 400 * time.Millisecond
	defaultDumpMaxDuration = 3 * time.Second
)

// OpenOptions configures a [Client] before it's opened. The zero
// value of every field is replaced by a sensible default; callers
// only need to set fields they want to override.
type OpenOptions struct {
	// PortMatch is a substring searched for in the system MIDI port
	// names to identify the controller. Defaults to "MC8 Pro Port 1".
	PortMatch string

	// SettleTime is how long to wait after sending session-open
	// before issuing the first request. The MC8 Pro silently ignores
	// requests during this window. Defaults to 1.2s.
	SettleTime time.Duration

	// PingTimeout is the maximum wait for a single Ping reply.
	// Defaults to 1s.
	PingTimeout time.Duration

	// FrameTimeout is the maximum wait for any single request/reply
	// pair other than ping. Defaults to 2s.
	FrameTimeout time.Duration

	// DumpQuietPeriod is how long the dump collector waits with no
	// incoming frames before considering the device done streaming.
	// Defaults to 400ms. Lower values return sooner but risk
	// truncating a slow trailing frame; higher values are safer but
	// add fixed latency to every Open and SelectBank call.
	DumpQuietPeriod time.Duration

	// DumpMaxDuration is a hard ceiling on how long the dump
	// collector runs regardless of quiet-period detection. Defaults
	// to 3s. This bounds the pathological case where the device
	// keeps trickling small frames indefinitely.
	DumpMaxDuration time.Duration

	// Logger receives structured log events from the SDK. If nil, a
	// no-op logger is used. Pass slog.Default() to send to the
	// program's default handler, or any other *slog.Logger.
	Logger *slog.Logger
}

func (o *OpenOptions) applyDefaults() {
	if o.PortMatch == "" {
		o.PortMatch = defaultPortMatch
	}
	if o.SettleTime == 0 {
		o.SettleTime = defaultSettleTime
	}
	if o.PingTimeout == 0 {
		o.PingTimeout = defaultPingTimeout
	}
	if o.FrameTimeout == 0 {
		o.FrameTimeout = defaultFrameTimeout
	}
	if o.DumpQuietPeriod == 0 {
		o.DumpQuietPeriod = defaultDumpQuietPeriod
	}
	if o.DumpMaxDuration == 0 {
		o.DumpMaxDuration = defaultDumpMaxDuration
	}
	if o.Logger == nil {
		o.Logger = discardLogger()
	}
}

// Client is one live session with one Morningstar MC8 Pro. It owns
// the MIDI port, the inbound frame router, and the device-state
// snapshot collected at open time (plus updates from subsequent
// SelectBank / Refresh calls).
//
// Concurrency: methods are safe to call from multiple goroutines.
// State access is protected by stateMu; router access by the router's
// own mutex.
//
// Lifecycle:
//
//	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{...})
//	if err != nil { return err }
//	defer client.Close()
//	// ... use client ...
//
// Forgetting Close() leaves the device stuck in edit mode until a
// power cycle or another tool sends the session-close. Always defer.
type Client struct {
	opts   OpenOptions
	log    *slog.Logger
	port   *midiPort
	router *router

	stateMu sync.RWMutex
	state   model.State
}

// Open establishes a session with the first Morningstar controller
// matching opts.PortMatch. It performs the full handshake:
//
//  1. Open MIDI ports
//  2. Send session-open (cmd 00 1B)
//  3. Wait SettleTime for the device to enter edit mode
//  4. Request firmware version (cmd 00 2C) and parse the reply (cmd 11 03)
//  5. Return a ready-to-use *Client
//
// On any failure during this sequence, partially-acquired resources
// are released and a clean session-close (00 1C) is attempted before
// returning the error. The device should never be left stuck in edit
// mode by a failed Open.
//
// The provided context is honored throughout open: cancellation
// aborts whatever step is in flight and triggers the same cleanup.
func Open(ctx context.Context, opts OpenOptions) (*Client, error) {
	opts.applyDefaults()
	log := opts.Logger.With(slog.String("component", "client"))

	c := &Client{
		opts:   opts,
		log:    log,
		router: newRouter(log),
	}

	port, err := openMIDIPort(opts.PortMatch, c.handleFrame, opts.Logger)
	if err != nil {
		return nil, err
	}
	c.port = port

	// From here on, any failure must clean up the port AND attempt a
	// session-close so the device doesn't stay in edit mode.
	cleanup := func() {
		log.Warn("open failed, cleaning up")
		_ = c.sendSessionClose()
		_ = c.port.Close()
	}

	// Subscribe to EVERY incoming frame for the dump-collection
	// window. We register BEFORE sending session-open because the
	// device often emits an unsolicited 00 7D session-active ping
	// and other frames before the settle time elapses.
	dumpCh, dumpCancel := c.router.subscribe(nil)
	defer dumpCancel()

	if err := c.sendSessionOpen(); err != nil {
		cleanup()
		return nil, err
	}

	log.Info("waiting for device to settle", slog.Duration("settle", opts.SettleTime))
	settleTimer := time.NewTimer(opts.SettleTime)
	select {
	case <-settleTimer.C:
	case <-ctx.Done():
		settleTimer.Stop()
		cleanup()
		return nil, ctx.Err()
	}

	// Fire the canonical request cascade from editor.js:90802 to
	// ensure every sub-system responds even if the device's
	// proactive dump omits something.
	if err := c.sendInitRequests(ctx); err != nil {
		cleanup()
		return nil, err
	}

	// Collect frames until the device goes quiet.
	state, err := c.drainDump(ctx, dumpCh)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("mc8pro: collect dump: %w", err)
	}

	c.stateMu.Lock()
	c.state = state
	c.stateMu.Unlock()

	log.Info("session opened",
		slog.Int("model", state.Device.Model),
		slog.String("firmware", state.Device.Firmware.String()),
		slog.String("serial", fmt.Sprintf("% X", state.Device.Serial)),
		slog.Int("current_bank", state.CurrentBank),
		slog.String("bank_name", state.Bank.BankName),
	)

	return c, nil
}

// Close ends the session: sends the session-close command (cmd 00 1C),
// closes the MIDI port, and wakes any pending waiters. Idempotent and
// safe to call multiple times.
//
// The session-close is sent on a best-effort basis. If it fails (port
// already disconnected, etc.), the error is logged but not returned;
// Close prioritizes resource cleanup over error reporting.
func (c *Client) Close() error {
	if c == nil || c.port == nil {
		return nil
	}
	c.log.Info("closing session")

	if err := c.sendSessionClose(); err != nil {
		c.log.Warn("session close send failed", slog.String("err", err.Error()))
	}

	if c.router != nil {
		c.router.closeAll()
	}

	err := c.port.Close()
	c.port = nil
	return err
}

// Device returns the device metadata captured during Open (model,
// firmware version, serial number). Safe to call concurrently.
func (c *Client) Device() Device {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state.Device
}

// State returns a deep copy of the current device state: firmware,
// bank names, currently-loaded bank contents, and any opaque
// passthrough data. The returned value is owned by the caller and
// can be mutated without affecting the Client's internal state.
//
// Populated at Open time and updated by SelectBank.
func (c *Client) State() State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state.Clone()
}

// WritePreset writes one preset to the device. The preset is
// identified by bank index (0..127) and preset index (0..23 for
// main presets). The full preset data — messages, names, and whatever
// metadata is populated — is sent in a single SysEx frame.
//
// This corresponds to editor.js:85910 sendFullPresetData, which
// sends cmd 06 11 with args [bank, preset, isExp, 0] and the
// EncodePresetFrame output as payload.
//
// The write is fire-and-forget at the protocol level: the device
// does not send an acknowledgment. The editor itself follows the same
// pattern — it sends the frame, updates its UI, and moves on.
//
// After a successful write, the local State is NOT automatically
// updated. Call SelectBank to re-read the bank if you need to verify
// the change landed. This deliberate choice keeps WritePreset simple
// and avoids second-guessing the device about what it actually stored
// (the device may normalize some fields during the write).
//
// For expression presets, use WriteExpPreset instead.
func (c *Client) WritePreset(ctx context.Context, bank, presetNum int, p Preset) error {
	if bank < 0 || bank > 127 {
		return fmt.Errorf("mc8pro: bank index %d out of range 0..127", bank)
	}
	if presetNum < 0 || presetNum > 23 {
		return fmt.Errorf("mc8pro: preset index %d out of range 0..23", presetNum)
	}

	// Ensure the Preset struct's addressing matches the explicit args.
	p.BankNum = bank
	p.PresetNum = presetNum
	p.IsExp = false

	payload := sysex.EncodePresetFrame(p)
	frame := sysex.Build(
		sysex.Cmd1Write,
		sysex.CmdWritePreset,
		[4]byte{byte(bank), byte(presetNum), 0x00, 0x00}, // isExp=0
		payload,
	)

	c.log.Info("writing preset",
		slog.Int("bank", bank),
		slog.Int("preset", presetNum),
		slog.Int("payload_bytes", len(payload)),
	)

	if err := c.port.Send(frame); err != nil {
		return fmt.Errorf("mc8pro: send write-preset: %w", err)
	}
	return nil
}

// WriteExpPreset writes one expression preset to the device. This is
// functionally identical to WritePreset but with the isExp flag set
// to 1 in the frame args. Expression presets are indexed 0..3 within
// each bank and live in Bank.ExpPresetArray.
func (c *Client) WriteExpPreset(ctx context.Context, bank, expPresetNum int, p Preset) error {
	if bank < 0 || bank > 127 {
		return fmt.Errorf("mc8pro: bank index %d out of range 0..127", bank)
	}
	if expPresetNum < 0 || expPresetNum > 3 {
		return fmt.Errorf("mc8pro: expression preset index %d out of range 0..3", expPresetNum)
	}

	p.BankNum = bank
	p.PresetNum = expPresetNum
	p.IsExp = true

	payload := sysex.EncodePresetFrame(p)
	frame := sysex.Build(
		sysex.Cmd1Write,
		sysex.CmdWritePreset,
		[4]byte{byte(bank), byte(expPresetNum), 0x01, 0x00}, // isExp=1
		payload,
	)

	c.log.Info("writing expression preset",
		slog.Int("bank", bank),
		slog.Int("exp_preset", expPresetNum),
		slog.Int("payload_bytes", len(payload)),
	)

	if err := c.port.Send(frame); err != nil {
		return fmt.Errorf("mc8pro: send write-exp-preset: %w", err)
	}
	return nil
}

// WriteBankConfig writes the bank-level configuration for one bank:
// name, description, config flags (clear toggle, colours, page limit),
// and the 32 bank-level messages. Maps to cmd 06 12.
//
// This is fire-and-forget like WritePreset — the device does not send
// an acknowledgment. The caller should allow ~1.5s for flash commit
// before re-reading via SelectBank to verify.
func (c *Client) WriteBankConfig(ctx context.Context, bank int, b Bank) error {
	if bank < 0 || bank > 127 {
		return fmt.Errorf("mc8pro: bank index %d out of range 0..127", bank)
	}

	b.BankNumber = bank

	payload := sysex.EncodeBankMetaFrame(b)
	frame := sysex.Build(
		sysex.Cmd1Write,
		sysex.CmdWriteBank,
		[4]byte{byte(bank), 0x00, 0x00, 0x00},
		payload,
	)

	c.log.Info("writing bank config",
		slog.Int("bank", bank),
		slog.Int("payload_bytes", len(payload)),
	)

	if err := c.port.Send(frame); err != nil {
		return fmt.Errorf("mc8pro: send write-bank-config: %w", err)
	}
	return nil
}

// sendUpload wraps a controller-settings write in the
// startTransmission / endTransmission envelope that the editor uses
// for all 04 xx writes. cmd2 is the specific upload command (e.g.
// CmdUploadControllerCfg = 0x02). args are the 4-byte arguments
// (zero for most commands). payload is the encoded data.
func (c *Client) sendUpload(cmd2 byte, args [4]byte, payload []byte) error {
	// startTransmission
	start := sysex.Build(sysex.Cmd1Upload, sysex.CmdUploadStart, sysex.NoArgs, nil)
	if err := c.port.Send(start); err != nil {
		return fmt.Errorf("mc8pro: send startTransmission: %w", err)
	}

	// data frame
	data := sysex.Build(sysex.Cmd1Upload, cmd2, args, payload)
	if err := c.port.Send(data); err != nil {
		return fmt.Errorf("mc8pro: send upload cmd2=0x%02X: %w", cmd2, err)
	}

	// endTransmission
	end := sysex.Build(sysex.Cmd1Upload, sysex.CmdUploadEnd, sysex.NoArgs, nil)
	if err := c.port.Send(end); err != nil {
		return fmt.Errorf("mc8pro: send endTransmission: %w", err)
	}
	return nil
}

// WriteControllerConfig writes the global device settings. Maps to
// cmd 04 02 (wrapped in startTransmission/endTransmission).
func (c *Client) WriteControllerConfig(ctx context.Context, cfg ControllerConfig) error {
	payload := sysex.EncodeControllerConfig(cfg)
	c.log.Info("writing controller config", slog.Int("payload_bytes", len(payload)))
	return c.sendUpload(sysex.CmdUploadControllerCfg, sysex.NoArgs, payload)
}

// WriteWaveformEngines writes the waveform engine table. Maps to
// cmd 04 05.
func (c *Client) WriteWaveformEngines(ctx context.Context, engines []WaveformEngine) error {
	payload := sysex.EncodeWaveformEngines(engines)
	c.log.Info("writing waveform engines", slog.Int("count", len(engines)))
	return c.sendUpload(sysex.CmdUploadWaveform, sysex.NoArgs, payload)
}

// WriteSequencerEngines writes the sequencer engine table. Maps to
// cmd 04 06.
func (c *Client) WriteSequencerEngines(ctx context.Context, engines []SequencerEngine) error {
	payload := sysex.EncodeSequencerEngines(engines)
	c.log.Info("writing sequencer engines", slog.Int("count", len(engines)))
	return c.sendUpload(sysex.CmdUploadSequencer, sysex.NoArgs, payload)
}

// WriteOmniports writes the omniport / expression input table. Maps
// to cmd 04 08.
func (c *Client) WriteOmniports(ctx context.Context, ports []OmniportInput) error {
	payload := sysex.EncodeOmniports(ports)
	c.log.Info("writing omniports", slog.Int("count", len(ports)))
	return c.sendUpload(sysex.CmdUploadOmniports, sysex.NoArgs, payload)
}

// WriteResistorLadder writes the aux switch calibration table. Maps
// to cmd 04 0B.
func (c *Client) WriteResistorLadder(ctx context.Context, switches []ResistorLadderSwitch) error {
	payload := sysex.EncodeResistorLadder(switches)
	c.log.Info("writing resistor ladder", slog.Int("count", len(switches)))
	return c.sendUpload(sysex.CmdUploadResistorLadder, sysex.NoArgs, payload)
}

// WriteMidiClockSlots writes the MIDI clock BPM presets. Maps to
// cmd 04 0C.
func (c *Client) WriteMidiClockSlots(ctx context.Context, slots []MidiClockSlot) error {
	payload := sysex.EncodeMidiClockSlots(slots)
	c.log.Info("writing midi clock slots", slog.Int("count", len(slots)))
	return c.sendUpload(sysex.CmdUploadMidiClockSlots, sysex.NoArgs, payload)
}

// WriteMidiEvents writes the MIDI event processor remap table. Maps
// to cmd 04 0A.
func (c *Client) WriteMidiEvents(ctx context.Context, ep MidiEventProcessor) error {
	payload := sysex.EncodeMidiEvents(ep)
	c.log.Info("writing midi events", slog.Int("payload_bytes", len(payload)))
	return c.sendUpload(sysex.CmdUploadEventProcessor, sysex.NoArgs, payload)
}

// WriteMidiChannels writes the MIDI channel names, remap, and
// attribute configuration. Maps to cmd 04 03 (row-framed).
func (c *Client) WriteMidiChannels(ctx context.Context, channels [16]MidiChannel) error {
	payload := sysex.EncodeMidiChannels(channels)
	c.log.Info("writing midi channels", slog.Int("payload_bytes", len(payload)))
	return c.sendUpload(sysex.CmdUploadMidiChannels, sysex.NoArgs, payload)
}

// WriteBankArrangement writes the bank ordering configuration. Maps
// to cmd 04 04.
func (c *Client) WriteBankArrangement(ctx context.Context, ba BankArrangement) error {
	payload := sysex.EncodeBankArrangement(ba)
	c.log.Info("writing bank arrangement", slog.Int("payload_bytes", len(payload)))
	return c.sendUpload(sysex.CmdUploadBankArrange, sysex.NoArgs, payload)
}

// RearrangePresets reorders the presets within the current bank. The
// order slice contains preset indices in the desired new order — e.g.
// [6, 0, 1, 2, 3, 4, 5, 7, ...] moves preset G to position A and
// shifts the rest. The device handles the data shuffling internally.
// Maps to cmd 04 09.
//
// The slice length should match the number of preset slots for the
// device (24 for MC8 Pro main presets).
func (c *Client) RearrangePresets(ctx context.Context, order []int) error {
	payload := make([]byte, len(order))
	for i, idx := range order {
		payload[i] = byte(idx)
	}
	c.log.Info("rearranging presets", slog.Int("count", len(order)))
	return c.sendUpload(sysex.CmdUploadPresetRearrange, sysex.NoArgs, payload)
}

// SwapPreset triggers the device-side preset swap for a main preset
// slot. The editor's "swap preset" feature works by: (1) user copies
// a preset in the editor UI (stored client-side), (2) editor sends
// cmd 0,24 to tell the device to swap. This is NOT a clipboard
// copy/paste — the data movement is handled by the editor writing
// the preset data, then sending this command to signal completion.
// Maps to cmd 00 18 (sendSysexFunction(0, 24)).
//
// Note: for SDK callers, ReadBank + WritePreset achieves the same
// result with full control over the data.
func (c *Client) SwapPreset(ctx context.Context, presetNum int) error {
	if presetNum < 0 || presetNum > 23 {
		return fmt.Errorf("mc8pro: preset index %d out of range 0..23", presetNum)
	}
	frame := sysex.Build(sysex.Cmd1General, sysex.CmdSwapPreset, [4]byte{byte(presetNum)}, nil)
	c.log.Info("swapping preset", slog.Int("preset", presetNum))
	return c.port.Send(frame)
}

// SwapExpPreset triggers the device-side preset swap for an
// expression preset slot. Same mechanics as SwapPreset but for
// expression presets (indices 0..3).
// Maps to cmd 00 19 (sendSysexFunction(0, 25)).
func (c *Client) SwapExpPreset(ctx context.Context, expNum int) error {
	if expNum < 0 || expNum > 3 {
		return fmt.Errorf("mc8pro: expression preset index %d out of range 0..3", expNum)
	}
	frame := sysex.Build(sysex.Cmd1General, sysex.CmdSwapExpPreset, [4]byte{byte(expNum)}, nil)
	c.log.Info("swapping expression preset", slog.Int("exp", expNum))
	return c.port.Send(frame)
}

// EngagePreset remotely triggers a preset switch press, as if the
// user physically pressed the footswitch. Maps to cmd 00 1D
// (ENGAGE_PRESET, sendSysexFunction(0, 29, preset)).
func (c *Client) EngagePreset(ctx context.Context, presetNum int) error {
	if presetNum < 0 || presetNum > 23 {
		return fmt.Errorf("mc8pro: preset index %d out of range 0..23", presetNum)
	}
	frame := sysex.Build(sysex.Cmd1General, sysex.CmdReqEngagePreset, [4]byte{byte(presetNum)}, nil)
	c.log.Info("engaging preset", slog.Int("preset", presetNum))
	return c.port.Send(frame)
}

// EngageExpression remotely triggers an expression preset. Maps to
// cmd 00 1E (ENGAGE_EXP, sendSysexFunction(0, 30, exp)).
func (c *Client) EngageExpression(ctx context.Context, expNum int) error {
	if expNum < 0 || expNum > 3 {
		return fmt.Errorf("mc8pro: expression preset index %d out of range 0..3", expNum)
	}
	frame := sysex.Build(sysex.Cmd1General, sysex.CmdReqEngageExp, [4]byte{byte(expNum)}, nil)
	c.log.Info("engaging expression", slog.Int("exp", expNum))
	return c.port.Send(frame)
}

// ToggleLooperMode toggles the device's looper mode on or off. Maps
// to cmd 00 2F (TOGGLE_LOOPER_MODE, sendSysexFunction(0, 47)).
func (c *Client) ToggleLooperMode(ctx context.Context) error {
	frame := sysex.Build(sysex.Cmd1General, sysex.CmdToggleLooperMode, sysex.NoArgs, nil)
	c.log.Info("toggling looper mode")
	return c.port.Send(frame)
}

// Subscribe registers a listener for incoming SysEx frames that match
// the provided filter function. Every frame for which filter returns
// true is delivered to the returned channel. A nil filter matches all
// frames.
//
// The caller MUST call cancel when done to release the subscription.
// The channel is closed when cancel is called or when the Client is
// closed.
//
// This exposes the internal router for external consumers that need
// real-time device events — for example, a GUI that wants to react
// when the user physically switches banks on the device.
//
//	ch, cancel := client.Subscribe(func(f sysex.Frame) bool {
//	    return f.Cmd1 == 0x06 && f.Cmd2 == 0x01 // preset data frames
//	})
//	defer cancel()
//	for frame := range ch {
//	    // handle frame
//	}
func (c *Client) Subscribe(filter func(sysex.Frame) bool) (<-chan sysex.Frame, func()) {
	return c.router.subscribe(filter)
}

// SelectBank changes the device's currently-focused bank to the
// given index (0..127) and collects the resulting dump into State.
// The device's LCD updates to show the new bank.
//
// Internally this sends cmd 00 1F <bank> 01 00 00 — the
// sendEditorBankChange command from editor.js:90754, which is what
// the bank dropdown in the official editor actually uses (NOT
// onSelectBank/(0, 22), which is a separate code path with an
// unknown purpose). ARG2 = 1 signals "not an expression preset";
// pass 0 there if the SDK ever exposes expression-preset selection.
//
// After sending the select command, the device dumps the three
// bank-scoped frames (06 01, 06 02, 09 01) representing the newly
// focused bank. The shorter DumpQuietPeriod (400ms default)
// typically means this call completes in well under a second.
//
// On failure, the previous state is preserved (atomic update).
func (c *Client) SelectBank(ctx context.Context, bank int) error {
	if bank < 0 || bank > 127 {
		return fmt.Errorf("mc8pro: bank index %d out of range 0..127", bank)
	}
	c.log.Info("selecting bank", slog.Int("bank", bank))

	// Subscribe before sending so we capture the immediate response.
	dumpCh, dumpCancel := c.router.subscribe(nil)
	defer dumpCancel()

	// cmd 00 1F <bank> 01 00 00 — sendEditorBankChange at
	// editor.js:90754. The bank index goes in ARG1 (byte 8),
	// ARG2 = 1 means "main preset" (not expression).
	frame := sysex.Build(sysex.Cmd1General, 0x1F, [4]byte{byte(bank), 0x01, 0x00, 0x00}, nil)
	if err := c.port.Send(frame); err != nil {
		return fmt.Errorf("mc8pro: send select-bank: %w", err)
	}

	// Seed state from current state — device-wide fields (firmware,
	// bank names) persist across bank switches. For different banks,
	// ingestFrame resets Bank via its bank-mismatch logic.
	c.stateMu.RLock()
	newState := c.state.Clone()
	c.stateMu.RUnlock()

	// Drain the bank-switch response (~3 frames: 06 01 + 06 02 +
	// 09 01). This gives us bank name, shortnames, and one preset.
	if err := c.drainDumpInto(ctx, dumpCh, &newState); err != nil {
		return fmt.Errorf("mc8pro: collect bank dump: %w", err)
	}

	c.stateMu.Lock()
	c.state = newState
	c.stateMu.Unlock()

	c.log.Info("bank switched, now reading full bank data",
		slog.Int("bank", newState.CurrentBank),
		slog.String("bank_name", newState.Bank.BankName),
	)

	// Brief settle before requesting backup — the device needs a
	// moment after processing a bank switch before it can handle
	// the backup protocol. Without this, the device may crash and
	// restart (observed empirically).
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Automatically read the full bank — SelectBank and ReadBank
	// are intrinsically linked. There's no use case for navigating
	// to a bank without loading its complete preset data.
	return c.ReadBank(ctx)
}

// ReadBank requests a full dump of the currently-selected bank from
// the device. This populates State.Bank with ALL 24 main presets
// and 4 expression presets — unlike SelectBank which only gets
// metadata and one preset.
//
// Internally this sends the "bankNewProtocol" backup request
// (cmd 07 00, arg=50 — editor.js:55556) and waits for the device
// to stream all preset frames. The stream is bracketed by a
// header frame (07 00 arg=0) and a terminator frame (07 00 arg=1),
// so completion is deterministic — no quiet-period guessing.
//
// The device briefly enters a visible "data dump" mode on the LCD
// during the transfer (~3 seconds observed). This is a single mode
// change, NOT per-preset cycling.
//
// Typical usage:
//
//	client.SelectBank(ctx, 0)   // navigate to bank 0 (fast, ~400ms)
//	client.ReadBank(ctx)        // full dump of bank 0 (complete, ~3s)
//	state := client.State()     // all 24+4 presets populated
//
// On failure, the previous state is preserved.
func (c *Client) ReadBank(ctx context.Context) error {
	c.log.Info("requesting full bank backup")

	ch, cancel := c.router.subscribe(nil)
	defer cancel()

	// Send the "bankNewProtocol" backup request.
	// From editor.js:55556: sendSysexFunction(7, 0, 50)
	//
	// IMPORTANT: The device validates byte 4 (model ID) for backup
	// commands differently than for other commands. The editor sets
	// DEVICE_MODEL_ID from the ping reply's byte 8 (session state
	// flag = 1), so the editor sends byte 4 = 0x01 — NOT 0x08
	// (the actual MC8 Pro model number). If we send 0x08, the
	// device returns a truncated dump (bank metadata only, no
	// preset data). We must patch byte 4 to match what the editor
	// sends. See new_backup_log.txt for the evidence.
	backupReq := sysex.Build(
		sysex.Cmd1Backup,
		sysex.CmdBackupHeader,
		[4]byte{sysex.CmdBackupRequestSingleBank, 0, 0, 0},
		nil,
	)
	// The editor sets DEVICE_MODEL_ID = 1 (from ping reply byte 8).
	// Testing with 0x01 previously showed 0 frames, but the device
	// may have been in a bad state. Retrying with 0x01 to match
	// the editor exactly.
	backupReq[sysex.OffsetModel] = 0x00
	backupReq[len(backupReq)-2] = sysex.Checksum(backupReq)
	c.log.Info("backup request bytes",
		slog.String("hex", fmt.Sprintf("% X", backupReq)),
		slog.Int("len", len(backupReq)),
	)
	if err := c.port.Send(backupReq); err != nil {
		return fmt.Errorf("mc8pro: send backup request: %w", err)
	}

	// Collect frames until we see the terminator: 07 00 with arg1=1.
	// The stream structure is:
	//   07 00 arg=0        header (dump start)
	//   07 02              bank metadata
	//   07 01 × 24         main presets
	//   07 01 × 4          expression presets
	//   07 00 arg=1        terminator (dump complete)
	c.stateMu.RLock()
	newState := c.state.Clone()
	c.stateMu.RUnlock()

	hardDeadline := time.NewTimer(30 * time.Second)
	defer hardDeadline.Stop()

	frameCount := 0
	for {
		select {
		case frame, ok := <-ch:
			if !ok {
				return errors.New("mc8pro: subscription closed during bank read")
			}
			frameCount++
			c.log.Debug("ReadBank frame received",
				slog.Int("n", frameCount),
				slog.String("cmd1", fmt.Sprintf("%02X", frame.Cmd1)),
				slog.String("cmd2", fmt.Sprintf("%02X", frame.Cmd2)),
				slog.Int("payload", len(frame.Payload)),
			)

			// ACK every frame. The device expects an acknowledgment
			// after each backup frame; without it, it waits ~8s per
			// frame and produces a truncated dump. The ACK is
			// sendSysexFunction(0, 127, checksum) where checksum is
			// the received frame's checksum byte.
			// See editor.js:18556 addMIDIMessageToBuffer.
			c.sendAck(frame)

			// Check for the terminator: 07 00 with arg1 = 1.
			if frame.Cmd1 == sysex.Cmd1Backup && frame.Cmd2 == sysex.CmdBackupHeader && frame.Args[0] == 1 {
				c.log.Info("bank backup complete",
					slog.Int("frames", frameCount),
				)
				c.stateMu.Lock()
				c.state = newState
				c.stateMu.Unlock()
				return nil
			}

			// Ingest all non-terminator frames.
			ingestFrame(&newState, frame, c.log)

		case <-hardDeadline.C:
			return fmt.Errorf("mc8pro: bank backup timed out after 10s (%d frames received)", frameCount)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// RestoreBank uploads a complete bank to the device using the restore
// protocol — the inverse of the ReadBank backup protocol. The device
// drives the pace: after each frame we send, it ACKs with
// (07, 00, arg=33) before we send the next. This avoids the flash
// commit timing issues that plague fire-and-forget writes.
//
// The protocol (from editor.js:55693-55957):
//  1. SDK → Device: (07, 00, arg=48, 0) — start single-bank restore
//  2. Device → SDK: (07, 00, arg=33) — ready for next frame
//  3. SDK → Device: bank metadata (07, 16, payload)
//  4. Device → SDK: (07, 00, arg=33)
//  5. SDK → Device: preset 0 (07, 17, presetNum, 0, payload)
//     ... repeat for all 24 presets and 4 expression presets ...
//  6. SDK → Device: (07, 00, arg=49, 0) — restore complete
//  7. Device → SDK: (07, 00, arg=17) — "completed" / (07, 00, arg=3) — "failed"
func (c *Client) RestoreBank(ctx context.Context, bank Bank) error {
	c.log.Info("starting bank restore",
		slog.Int("bank", bank.BankNumber),
		slog.String("name", bank.BankName),
	)

	// Subscribe to backup-family responses (07 xx).
	ch, cancel := c.router.subscribe(func(f sysex.Frame) bool {
		return f.Cmd1 == sysex.Cmd1Backup && f.Cmd2 == sysex.CmdBackupHeader
	})
	defer cancel()

	// Step 1: tell the device to start a single-bank restore.
	startReq := sysex.Build(
		sysex.Cmd1Backup,
		sysex.CmdBackupHeader,
		[4]byte{sysex.CmdRestoreStartSingle, 0, 0, 0},
		nil,
	)
	startReq[sysex.OffsetModel] = 0x08
	startReq[len(startReq)-2] = sysex.Checksum(startReq)
	if err := c.port.Send(startReq); err != nil {
		return fmt.Errorf("mc8pro: send restore-start: %w", err)
	}

	// Wait for device ready ACK (07, 00, arg=33).
	if err := c.waitRestoreReady(ctx, ch); err != nil {
		return fmt.Errorf("mc8pro: restore start handshake: %w", err)
	}

	// Step 2: build the frame queue — bank metadata first, then
	// presets, then expression presets. Matches editor.js:55697-55729.
	type restoreFrame struct {
		name    string
		frame   []byte
	}
	var queue []restoreFrame

	// Bank metadata.
	bankPayload := sysex.EncodeBankMetaFrame(bank)
	bankFrame := sysex.Build(sysex.Cmd1Backup, sysex.CmdRestoreBankMeta, sysex.NoArgs, bankPayload)
	bankFrame[sysex.OffsetModel] = 0x08
	bankFrame[len(bankFrame)-2] = sysex.Checksum(bankFrame)
	queue = append(queue, restoreFrame{"bank-meta", bankFrame})

	// 24 main presets.
	for i, p := range bank.PresetArray {
		p.BankNum = bank.BankNumber
		p.PresetNum = i
		p.IsExp = false
		payload := sysex.EncodePresetFrame(p)
		frame := sysex.Build(sysex.Cmd1Backup, sysex.CmdRestorePreset, [4]byte{byte(i), 0, 0, 0}, payload)
		frame[sysex.OffsetModel] = 0x08
		frame[len(frame)-2] = sysex.Checksum(frame)
		queue = append(queue, restoreFrame{fmt.Sprintf("preset-%d", i), frame})
	}

	// 4 expression presets.
	for i, p := range bank.ExpPresetArray {
		p.BankNum = bank.BankNumber
		p.PresetNum = i
		p.IsExp = true
		payload := sysex.EncodePresetFrame(p)
		frame := sysex.Build(sysex.Cmd1Backup, sysex.CmdRestoreExpPreset, [4]byte{byte(i), 0, 0, 0}, payload)
		frame[sysex.OffsetModel] = 0x08
		frame[len(frame)-2] = sysex.Checksum(frame)
		queue = append(queue, restoreFrame{fmt.Sprintf("exp-preset-%d", i), frame})
	}

	// Step 3: send each frame, waiting for device ACK between each.
	for n, qf := range queue {
		c.log.Debug("sending restore frame",
			slog.Int("n", n+1),
			slog.Int("total", len(queue)),
			slog.String("name", qf.name),
			slog.Int("bytes", len(qf.frame)),
		)
		if err := c.port.Send(qf.frame); err != nil {
			return fmt.Errorf("mc8pro: send restore frame %s: %w", qf.name, err)
		}

		// Wait for device ready (07, 00, arg=33) before sending next.
		if err := c.waitRestoreReady(ctx, ch); err != nil {
			return fmt.Errorf("mc8pro: restore ACK after %s: %w", qf.name, err)
		}
	}

	// Step 4: signal completion.
	completeReq := sysex.Build(
		sysex.Cmd1Backup,
		sysex.CmdBackupHeader,
		[4]byte{sysex.CmdRestoreComplete, 0, 0, 0},
		nil,
	)
	completeReq[sysex.OffsetModel] = 0x08
	completeReq[len(completeReq)-2] = sysex.Checksum(completeReq)
	if err := c.port.Send(completeReq); err != nil {
		return fmt.Errorf("mc8pro: send restore-complete: %w", err)
	}

	// Wait for the done signal. The device sends (07,00,arg=3) then
	// (07,00,arg=17) as part of the normal completion handshake —
	// arg=3 is NOT a failure. Confirmed by Protokol capture of a
	// successful editor restore. Loop until arg=17.
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case frame, ok := <-ch:
			if !ok {
				return errors.New("mc8pro: subscription closed during restore completion")
			}
			if frame.Args[0] == sysex.CmdRestoreDone {
				c.log.Info("bank restore complete", slog.Int("frames_sent", len(queue)))
				return nil
			}
			c.log.Debug("restore completion interim frame", slog.Int("arg", int(frame.Args[0])))
		case <-timeout.C:
			return errors.New("mc8pro: timed out waiting for restore completion")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// waitRestoreReady waits for the device's "ready for next frame" ACK:
// a (07, 00) frame with arg[0]=33. Returns an error on timeout or
// unexpected responses.
func (c *Client) waitRestoreReady(ctx context.Context, ch <-chan sysex.Frame) error {
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case frame, ok := <-ch:
			if !ok {
				return errors.New("subscription closed")
			}
			if frame.Args[0] == sysex.CmdRestoreReadyForNext {
				return nil
			}
			// The device may send other (07,00) frames during the
			// restore flow. Keep waiting for the ready signal.
			c.log.Debug("restore: skipping non-ready frame",
				slog.Int("arg", int(frame.Args[0])))
		case <-timeout.C:
			return errors.New("timed out waiting for device ready")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// sendAck sends an acknowledgment for a received frame. The device
// expects this after every SysEx frame during backup operations;
// without it, it waits ~8 seconds per frame and produces a truncated
// dump. The ACK format is sendSysexFunction(0, 127, checksum) where
// checksum is the received frame's checksum byte.
// See editor.js:18556 addMIDIMessageToBuffer.
func (c *Client) sendAck(frame sysex.Frame) {
	// The editor ACKs every received frame by echoing back its
	// checksum byte: sendSysexFunction(0, 127, checksum).
	// See editor.js:18556 and 18877.
	ack := sysex.Build(sysex.Cmd1General, sysex.CmdAck, [4]byte{frame.RawCksum, 0, 0, 0}, nil)
	if err := c.port.Send(ack); err != nil {
		c.log.Warn("failed to send ACK", slog.String("err", err.Error()))
	}
}

// sendInitRequests fires the canonical REQUEST_* cascade from
// editor.js:90802 requestControllerData(). Called from Open after
// the settle time elapses. Request spacing matches what we observed
// the editor doing; the device handles them fine with ~150ms gaps.
func (c *Client) sendInitRequests(ctx context.Context) error {
	for _, req := range initRequestSequence {
		frame := sysex.Build(sysex.Cmd1General, req.Cmd2, sysex.NoArgs, nil)
		if err := c.port.Send(frame); err != nil {
			return fmt.Errorf("mc8pro: send REQUEST_%s: %w", req.Name, err)
		}
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// drainDump collects inbound frames into a fresh State until the
// device goes quiet for DumpQuietPeriod or DumpMaxDuration elapses,
// whichever comes first. The caller supplies the subscription
// channel; this function is the consumer.
//
// See drainDumpInto for the mutable variant used by SelectBank to
// merge into an existing State seed.
func (c *Client) drainDump(ctx context.Context, ch <-chan sysex.Frame) (model.State, error) {
	state := model.NewState()
	if err := c.drainDumpInto(ctx, ch, &state); err != nil {
		return model.State{}, err
	}
	return state, nil
}

// drainDumpInto is the mutable form of drainDump. It ingests frames
// into the supplied State pointer until the device goes quiet. Used
// by SelectBank to preserve device-wide fields (firmware, bank
// names) across a bank switch.
func (c *Client) drainDumpInto(ctx context.Context, ch <-chan sysex.Frame, state *model.State) error {
	lastFrame := time.Now()
	frameCount := 0

	// Hard deadline bounds the worst case.
	hardDeadline := time.NewTimer(c.opts.DumpMaxDuration)
	defer hardDeadline.Stop()

	for {
		// Compute how long since the last frame arrived. If we've
		// been quiet for DumpQuietPeriod, we're done. Otherwise
		// wait up to (DumpQuietPeriod - quietSoFar) more for
		// another frame.
		quietSoFar := time.Since(lastFrame)
		if frameCount > 0 && quietSoFar >= c.opts.DumpQuietPeriod {
			c.log.Debug("dump drain complete (quiet period)",
				slog.Int("frames", frameCount),
				slog.Duration("quiet", quietSoFar))
			return nil
		}
		waitFor := c.opts.DumpQuietPeriod - quietSoFar
		if waitFor <= 0 {
			waitFor = c.opts.DumpQuietPeriod
		}

		select {
		case frame, ok := <-ch:
			if !ok {
				return errors.New("mc8pro: subscription closed during dump")
			}
			ingestFrame(state, frame, c.log)
			lastFrame = time.Now()
			frameCount++
		case <-time.After(waitFor):
			// Quiet period elapsed with no new frame since
			// lastFrame; loop back and the quietSoFar check will
			// return.
		case <-hardDeadline.C:
			c.log.Warn("dump drain hit hard deadline",
				slog.Int("frames", frameCount),
				slog.Duration("max", c.opts.DumpMaxDuration))
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Ping sends a ping (cmd 00 7D) and waits for the device's reply.
// Returns sessionActive=true if the device reports it's currently in
// edit mode, false otherwise.
//
// Useful as a liveness check and as a way to verify session state
// without consuming any other resources. The ping reply's byte 8 is
// what we read for the session-active flag.
//
// The ping has its own short timeout (opts.PingTimeout, default 1s)
// independent of any timeout on the supplied context. If ctx expires
// first, that takes precedence.
func (c *Client) Ping(ctx context.Context) (sessionActive bool, err error) {
	frame, err := c.exchange(ctx, sysex.Cmd1General, sysex.CmdPing, c.opts.PingTimeout)
	if err != nil {
		return false, err
	}
	// Byte 8 of the ping reply (= ARG1, the first function-id arg)
	// holds the session-state flag: 1 = active, 0 = inactive.
	return frame.Args[0] == 1, nil
}

// handleFrame is the callback invoked by the MIDI shim for every
// received SysEx frame. It parses the frame and dispatches it
// through the router. Frames that fail to parse or have no waiter
// are logged at debug level and discarded.
func (c *Client) handleFrame(raw []byte) {
	frame, err := sysex.Parse(raw)
	if err != nil {
		c.log.Warn("malformed inbound frame", slog.String("err", err.Error()))
		return
	}
	if !c.router.dispatch(frame) {
		c.log.Debug("unrouted frame",
			slog.String("cmd1", fmt.Sprintf("%02X", frame.Cmd1)),
			slog.String("cmd2", fmt.Sprintf("%02X", frame.Cmd2)),
		)
	}
}

// sendSessionOpen sends the cmd 00 1B handshake.
func (c *Client) sendSessionOpen() error {
	c.log.Debug("sending session open")
	return c.port.Send(sysex.Build(sysex.Cmd1General, sysex.CmdSessionOpen, sysex.NoArgs, nil))
}

// sendSessionClose sends the cmd 00 1C handshake. Used by both
// Close() and the cleanup path of a failed Open().
func (c *Client) sendSessionClose() error {
	c.log.Debug("sending session close")
	return c.port.Send(sysex.Build(sysex.Cmd1General, sysex.CmdSessionClose, sysex.NoArgs, nil))
}

// exchange is the core request/reply primitive: register a waiter
// for the expected reply command pair, send the request, await the
// reply or timeout. The reply command may differ from the request
// command (e.g. REQUEST_FIRMWARE_VERSION sends 00 2C and replies
// 11 03), so we register the waiter on the *reply* key, which is
// determined by lookup.
func (c *Client) exchange(ctx context.Context, reqCmd1, reqCmd2 byte, timeout time.Duration) (sysex.Frame, error) {
	replyCmd1, replyCmd2, ok := replyKeyFor(reqCmd1, reqCmd2)
	if !ok {
		return sysex.Frame{}, fmt.Errorf("mc8pro: no known reply key for cmd %02X %02X", reqCmd1, reqCmd2)
	}

	ch, cancel := c.router.expect(replyCmd1, replyCmd2)
	defer cancel()

	if err := c.port.Send(sysex.Build(reqCmd1, reqCmd2, sysex.NoArgs, nil)); err != nil {
		return sysex.Frame{}, fmt.Errorf("mc8pro: send: %w", err)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	select {
	case f, ok := <-ch:
		if !ok {
			return sysex.Frame{}, errors.New("mc8pro: client closed while awaiting reply")
		}
		return f, nil
	case <-deadline.C:
		return sysex.Frame{}, fmt.Errorf("mc8pro: timeout waiting for reply to cmd %02X %02X", reqCmd1, reqCmd2)
	case <-ctx.Done():
		return sysex.Frame{}, ctx.Err()
	}
}

// replyKeyFor maps a request command pair to its expected reply
// command pair. Most read commands have a 1:1 reply mapping that we
// know empirically from the protocol decoding work.
//
// As Phase 3 grows the set of supported reads, this table grows
// too. For now we only need ping and firmware.
func replyKeyFor(cmd1, cmd2 byte) (byte, byte, bool) {
	switch {
	case cmd1 == sysex.Cmd1General && cmd2 == sysex.CmdPing:
		// Ping echoes back its own command pair.
		return sysex.Cmd1General, sysex.CmdPing, true
	case cmd1 == sysex.Cmd1General && cmd2 == sysex.CmdReqControllerFirmwareVersion:
		// Firmware request → 11 03 reply.
		return 0x11, 0x03, true
	}
	return 0, 0, false
}
