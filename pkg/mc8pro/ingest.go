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
		// Bank metadata frame (06 02 / 07 02). Contains bank index,
		// bank-level messages, and bank name. For Phase 3 we decode
		// only the bank name (row tag 3, 32-char ASCII) and stash
		// the rest as raw.
		if name, ok := decodeBankNameFromMeta(frame.Payload); ok {
			state.Bank.BankName = name
		}
		state.Raw[key] = copyBytes(frame.Payload)

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
		// Bank names table (03 20). Contains 16-char bank name rows
		// for some range of banks. Populates State.BankNames at the
		// indices carried in the row tags.
		rows, err := sysex.ParseRows(frame.Payload)
		if err != nil {
			log.Warn("bank names parse failed", slog.String("err", err.Error()))
			return
		}
		for _, row := range rows {
			// The frame mixes row tags: 0x00..0x0F carry bank
			// names (16 chars each). Rows 0x10+ hold other data
			// we don't yet decode (stored in Raw via this case's
			// fallthrough-style stash below).
			if row.Tag > 0x0F {
				continue
			}
			if len(row.Data) < 1 {
				continue
			}
			// Row structure for tags 0x00..0x0F: length byte
			// followed by <length> bytes of name. But ParseRows
			// gives us just the data portion — wait, we need to
			// double-check this. The row wire format is
			// 7F <tag> <len> <data...> and ParseRows stores
			// row.Data as the <data...> bytes with len==row_len.
			// So for a 16-char bank name, row.Data is exactly 16
			// bytes of ASCII. No additional length prefix.
			state.BankNames[row.Tag] = trimASCII(row.Data)
		}
		// Also stash raw for round-trip fidelity of the unknown
		// trailing rows.
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

// decodeBankNameFromMeta pulls the bank name out of a 06 02 / 07 02
// payload by scanning for row tag 3. Returns (name, true) on success
// or ("", false) if no name row is present.
func decodeBankNameFromMeta(payload []byte) (string, bool) {
	rows, err := sysex.ParseRows(payload)
	if err != nil {
		return "", false
	}
	for _, row := range rows {
		if row.Tag == 0x03 && len(row.Data) > 0 {
			return trimASCII(row.Data), true
		}
	}
	return "", false
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
