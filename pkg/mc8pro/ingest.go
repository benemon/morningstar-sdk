package mc8pro

import (
	"log/slog"
	"strings"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// ingestFrame routes one inbound SysEx frame into the growing State,
// populating typed fields for frame types we understand and stashing
// opaque bytes in State.Raw for everything else.
//
// The ingester is deliberately tolerant: unknown commands are logged
// at debug level and stored in Raw, malformed decode attempts log a
// warning and leave State unchanged. This lets dump collection
// continue across firmware variations and unknown sub-sections
// without aborting.
//
// Called from Client.collectDump for every frame that arrives during
// the dump window.
func ingestFrame(state *model.State, frame sysex.Frame, log *slog.Logger) {
	key := uint16(frame.Cmd1)<<8 | uint16(frame.Cmd2)

	switch {
	case frame.Cmd1 == 0x11 && frame.Cmd2 == 0x03:
		// Firmware info frame (11 03). Populates State.Device.
		info, err := sysex.DecodeFirmwareFrame(frame)
		if err != nil {
			log.Warn("firmware decode failed", slog.String("err", err.Error()))
			return
		}
		state.Device = model.Device{
			Model: int(info.Model),
			Firmware: model.Version{
				Major: info.Major,
				Minor: info.Minor,
				Patch: info.Patch,
			},
			Serial: info.Serial,
		}

	case (frame.Cmd1 == 0x06 || frame.Cmd1 == 0x07) && frame.Cmd2 == 0x01:
		// Preset data frame (06 01 live edit or 07 01 backup).
		// The frame contains one preset's data including the bank
		// index in row 0 byte [0]. Populates the current preset
		// slot of State.Bank and updates State.CurrentBank.
		preset, err := sysex.DecodePresetFrame(frame.Payload)
		if err != nil {
			log.Warn("preset decode failed", slog.String("err", err.Error()))
			return
		}
		if preset.BankNum < 0 || preset.BankNum > 127 {
			log.Warn("preset frame has out-of-range bank index",
				slog.Int("bank", preset.BankNum))
			return
		}
		// The device always dumps the currently-focused bank. If
		// the frame's bank differs from our cached CurrentBank, the
		// device has just been navigated to a new bank — reset the
		// Bank field so we don't mix presets from different banks.
		if state.Bank.BankNumber != preset.BankNum {
			state.Bank = model.Bank{BankNumber: preset.BankNum}
		}
		state.CurrentBank = preset.BankNum

		// Route to the correct array based on the isExp flag
		// (header row byte [2]). Expression presets have indices
		// 0..3 and go into ExpPresetArray; main presets have
		// indices 0..23 and go into PresetArray.
		if preset.IsExp {
			if preset.PresetNum < 0 || preset.PresetNum >= len(state.Bank.ExpPresetArray) {
				log.Warn("expression preset frame has out-of-range index",
					slog.Int("preset", preset.PresetNum))
				return
			}
			state.Bank.ExpPresetArray[preset.PresetNum] = preset
		} else {
			if preset.PresetNum < 0 || preset.PresetNum >= len(state.Bank.PresetArray) {
				log.Warn("preset frame has out-of-range preset index",
					slog.Int("preset", preset.PresetNum))
				return
			}
			state.Bank.PresetArray[preset.PresetNum] = preset
		}

	case (frame.Cmd1 == 0x06 || frame.Cmd1 == 0x07) && frame.Cmd2 == 0x02:
		// Bank metadata frame (06 02 / 07 02). Contains bank number,
		// bank config flags, bank-level messages, bank name, and
		// bank description.
		bankMeta, err := sysex.DecodeBankMetaFrame(frame.Payload)
		if err != nil {
			log.Warn("bank metadata decode failed", slog.String("err", err.Error()))
			return
		}
		state.Bank.BankName = bankMeta.BankName
		state.Bank.BankDescription = bankMeta.BankDescription
		state.Bank.BankClearToggle = bankMeta.BankClearToggle
		state.Bank.ToDisplay = bankMeta.ToDisplay
		state.Bank.BackgroundColor = bankMeta.BackgroundColor
		state.Bank.TextColor = bankMeta.TextColor
		state.Bank.IsColorEnabled = bankMeta.IsColorEnabled
		state.Bank.PageLimit = bankMeta.PageLimit
		state.Bank.BankMsgArray = bankMeta.BankMsgArray
		// Also stash raw for round-trip fidelity of any fields we
		// might not yet decode.
		state.Raw[key] = copyBytes(frame.Payload)

	case frame.Cmd1 == 0x11 && frame.Cmd2 == 0x05:
		// Bank names (11 05). Paged: 8 bank names per frame, 16
		// frames total covering all 128 banks. Row tags 0–127 map
		// directly to bank indices. Each row is 32-byte ASCII.
		rows, err := sysex.ParseRows(frame.Payload)
		if err != nil {
			log.Warn("bank names parse failed", slog.String("err", err.Error()))
			return
		}
		for _, row := range rows {
			idx := int(row.Tag)
			if idx < 0 || idx >= len(state.BankNames) {
				continue
			}
			state.BankNames[idx] = trimASCII(row.Data)
		}

	case (frame.Cmd1 == 0x09 && frame.Cmd2 == 0x01) || (frame.Cmd1 == 0x11 && frame.Cmd2 == 0x04):
		// Preset shortnames frame (09 01 live / 11 04 mirror).
		// Contains up to 24 shortnames, one per preset slot in the
		// current bank, each as a 32-byte ASCII name row. Fans out
		// into State.Bank.PresetArray[i].ShortName.
		//
		// The frame args carry the bank index; we verify it matches
		// our CurrentBank but don't fail if it doesn't — the
		// ordering within a dump can be out of sync during settle.
		bank := int(frame.Args[0])
		if bank != state.CurrentBank && state.CurrentBank >= 0 && state.Bank.BankNumber == state.CurrentBank {
			log.Debug("shortnames frame bank mismatch",
				slog.Int("frame_bank", bank),
				slog.Int("current_bank", state.CurrentBank))
		}
		rows, err := sysex.ParseRows(frame.Payload)
		if err != nil {
			log.Warn("shortnames parse failed", slog.String("err", err.Error()))
			return
		}
		for _, row := range rows {
			idx := int(row.Tag)
			if idx < 0 || idx >= len(state.Bank.PresetArray) {
				continue
			}
			// Row length byte is the first byte of Data in our
			// row parser; the actual name follows. But ParseRows
			// already separates tag, length, and data bytes —
			// row.Data is just the name bytes.
			state.Bank.PresetArray[idx].ShortName = trimASCII(row.Data)
		}

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x20:
		// MIDI channel names + config (03 20). Previously
		// mis-identified as "bank names" — verified by finding
		// "Quad Cortex Mini" at tag 0x01 (channel 2).
		//
		// Tags 0x00–0x0F: 16-byte MIDI channel names (ASCII).
		// Tags 0x10–0x1F: 4-byte channel remap/port config.
		// Tags 0x20–0x2F: 16-byte channel attribute blocks.
		channels, err := sysex.DecodeMidiChannels(frame.Payload)
		if err != nil {
			log.Warn("midi channels decode failed", slog.String("err", err.Error()))
			state.Raw[key] = copyBytes(frame.Payload)
			return
		}
		state.MidiChannels = channels
		// Also stash raw for round-trip fidelity.
		state.Raw[key] = copyBytes(frame.Payload)

	case frame.Cmd1 == 0x11 && frame.Cmd2 == 0x00:
		// Global device settings (11 00). Purpose not yet fully
		// identified; stash as raw for now.
		state.Raw[key] = copyBytes(frame.Payload)

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x26:
		// Waveform engines (03 26).
		engines, err := sysex.DecodeWaveformEngines(frame.Payload)
		if err != nil {
			log.Warn("waveform engines decode failed", slog.String("err", err.Error()))
			return
		}
		state.WaveformEngines = engines

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x25:
		// Resistor ladder / aux switch calibration (03 25).
		switches, err := sysex.DecodeResistorLadder(frame.Payload)
		if err != nil {
			log.Warn("resistor ladder decode failed", slog.String("err", err.Error()))
			return
		}
		state.ResistorLadder = switches

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x28:
		// MIDI clock slots (03 28).
		slots, err := sysex.DecodeMidiClockSlots(frame.Payload)
		if err != nil {
			log.Warn("midi clock slots decode failed", slog.String("err", err.Error()))
			return
		}
		state.MidiClockSlots = slots

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x21:
		// Global controller settings (03 21, NOT bank arrangement
		// as previously mis-identified — verified by JSON correlation).
		cfg, err := sysex.DecodeControllerConfig(frame.Payload)
		if err != nil {
			log.Warn("controller config decode failed", slog.String("err", err.Error()))
			return
		}
		state.Controller = cfg

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x23:
		// Sequencer engines (03 23).
		engines, err := sysex.DecodeSequencerEngines(frame.Payload)
		if err != nil {
			log.Warn("sequencer engines decode failed", slog.String("err", err.Error()))
			return
		}
		state.SequencerEngines = engines

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x24:
		// Omniport / expression inputs (03 24).
		ports, err := sysex.DecodeOmniports(frame.Payload)
		if err != nil {
			log.Warn("omniports decode failed", slog.String("err", err.Error()))
			return
		}
		state.Omniports = ports

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x22:
		// MIDI event processor (03 22). 10-byte header + 128-byte remap.
		ep, err := sysex.DecodeMidiEvents(frame.Payload)
		if err != nil {
			log.Warn("midi events decode failed", slog.String("err", err.Error()))
			return
		}
		state.MidiEvents = ep

	case frame.Cmd1 == 0x03 && frame.Cmd2 == 0x27:
		// MIDI channel names (03 27). Layout not fully verified;
		// stash as raw for now.
		state.Raw[key] = copyBytes(frame.Payload)

	default:
		// Unknown frame type. Log and stash the payload opaquely.
		log.Debug("opaque frame stored in Raw",
			slog.String("cmd1", hex2(frame.Cmd1)),
			slog.String("cmd2", hex2(frame.Cmd2)),
			slog.Int("payload_len", len(frame.Payload)))
		state.Raw[key] = copyBytes(frame.Payload)
	}
}

// trimASCII interprets bytes as ASCII and trims trailing spaces.
// Morningstar pads name fields with 0x20 on the wire; the JSON form
// strips trailing spaces, so we do the same.
func trimASCII(b []byte) string {
	return strings.TrimRight(string(b), " ")
}

// copyBytes returns a fresh copy of src so the caller can store it
// without holding a reference to the inbound frame buffer (which may
// be reused by the MIDI driver).
func copyBytes(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

// hex2 formats a byte as two uppercase hex digits without using fmt.
func hex2(b byte) string {
	const hex = "0123456789ABCDEF"
	return string([]byte{hex[b>>4], hex[b&0xF]})
}
