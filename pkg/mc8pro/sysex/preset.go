package sysex

import (
	"fmt"
	"strings"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

// Row tags inside a per-preset frame (Cmd1=0x06 live writes,
// Cmd1=0x07 backup writes). These are the "row kind" tags; see
// CLAUDE.md for the full inventory.
const (
	PresetRowHeader     = 0x00 // 4 bytes: preset index + flags
	PresetRowMessage    = 0x01 // 23 bytes: one Message
	PresetRowShortName  = 0x02 // 32 bytes: shortName ASCII
	PresetRowToggleName = 0x03 // 32 bytes: toggleName ASCII
	PresetRowLongName   = 0x04 // 32 bytes: longName ASCII
	PresetRowConfig     = 0x05 // 32 bytes: flags + colors (layout TBD)
	PresetRowShiftName  = 0x06 // 32 bytes: shiftName ASCII
)

// presetHeaderLen is the fixed length of the PresetRowHeader row's
// data portion (row_len byte = 0x04).
const presetHeaderLen = 4

// messageRowLen is the fixed length of the PresetRowMessage row's
// data portion (row_len byte = 0x17 = 23).
const messageRowLen = 23

// nameRowLen is the fixed length of every name row (shortName,
// longName, etc). row_len byte = 0x20 = 32.
const nameRowLen = 32

// configRowLen is the fixed length of the preset config row. 0x20.
const configRowLen = 32

// DecodePresetFrame takes the payload of a frame whose command is
// (0x06 0x01) or (0x07 0x01) and returns a [model.Preset] populated
// from the row contents.
//
// Fields that we don't yet decode (LED colors, toggle flags, preset
// config flags from row 5) are left at their zero value. Message row
// data is decoded fully.
func DecodePresetFrame(payload []byte) (model.Preset, error) {
	rows, err := ParseRows(payload)
	if err != nil {
		return model.Preset{}, err
	}

	var p model.Preset
	var sawHeader bool

	for _, row := range rows {
		switch row.Tag {
		case PresetRowHeader:
			if len(row.Data) != presetHeaderLen {
				return model.Preset{}, fmt.Errorf("sysex: preset header row has %d bytes, want %d", len(row.Data), presetHeaderLen)
			}
			// Row data layout (verified across multi-bank captures;
			// see reference/capture-lifecycle-3events.md):
			// [0] = bank index (0..127; 127 = 0x7F is a valid literal
			//       bank, not a sentinel — confirmed by observing the
			//       device on bank 127 before editor connect)
			// [1] = preset index (0..23 for main, 0..3 for expression)
			// [2] = isExp flag (0 = main preset, 1 = expression preset)
			// [3] = unknown (seen 00)
			p.BankNum = int(row.Data[0])
			p.PresetNum = int(row.Data[1])
			p.IsExp = row.Data[2] != 0
			sawHeader = true
		case PresetRowMessage:
			if len(row.Data) != messageRowLen {
				return model.Preset{}, fmt.Errorf("sysex: message row has %d bytes, want %d", len(row.Data), messageRowLen)
			}
			msg := decodeMessageRow(row.Data)
			if msg.M < 0 || msg.M >= len(p.MsgArray) {
				return model.Preset{}, fmt.Errorf("sysex: message row slot index %d out of range", msg.M)
			}
			p.MsgArray[msg.M] = msg
		case PresetRowShortName:
			p.ShortName = decodeASCII(row.Data)
		case PresetRowToggleName:
			p.ToggleName = decodeASCII(row.Data)
		case PresetRowLongName:
			p.LongName = decodeASCII(row.Data)
		case PresetRowShiftName:
			p.ShiftName = decodeASCII(row.Data)
		case PresetRowConfig:
			// TODO: decode config row 5 (LED colors, toggle flags,
			// isExp, toToggle, etc). For now leave fields at zero
			// and rely on round-trip from JSON for these fields.
		default:
			// Unknown row tag: ignore. A future protocol version
			// may add rows; we don't want to reject them.
		}
	}

	if !sawHeader {
		return model.Preset{}, fmt.Errorf("sysex: preset frame missing header row")
	}
	return p, nil
}

// decodeMessageRow decodes the 23-byte data portion of a PresetRowMessage
// into a [model.Message]. The byte layout is from CLAUDE.md (verified by
// cross-reference against the JSON backup schema):
//
//	[0]      m  (slot index 0..31)
//	[1]      t  (message type)
//	[2]      data[0]  (CC# for CC type)
//	[3]      data[1]  (CC value for CC type)
//	[4]      data[2]
//	[5]      a  (action/trigger)
//	[6]      c  (MIDI channel, 1-indexed)
//	[7]      tg (toggle group)
//	[8..22]  data[3..17]
func decodeMessageRow(row []byte) model.Message {
	var m model.Message
	m.M = int(row[0])
	m.Type = int(row[1])
	m.Data[0] = int(row[2])
	m.Data[1] = int(row[3])
	m.Data[2] = int(row[4])
	m.Action = int(row[5])
	m.Channel = int(row[6])
	m.ToggleGroup = int(row[7])
	for i := 3; i < 18; i++ {
		m.Data[i] = int(row[5+i]) // [8..22] → data[3..17]
	}
	return m
}

// decodeASCII converts a space-padded ASCII row into a trimmed string.
// Morningstar stores all name fields as fixed-width space-padded
// buffers; the JSON form strips trailing spaces (verified against the
// editor's output — names like "Overdrive" are NOT padded in JSON).
func decodeASCII(b []byte) string {
	return strings.TrimRight(string(b), " ")
}

// EncodePresetFrame produces the payload bytes for a per-preset frame
// from a [model.Preset]. The inverse of DecodePresetFrame.
//
// Important caveat: this only encodes the fields that DecodePresetFrame
// decodes. Any field not round-tripped by the decoder (LED colors,
// config flags) will be emitted as zero in the resulting bytes. This
// means Encode(Decode(x)) == x holds, but Encode of a freshly-built
// Preset with custom colors will not yet produce a frame that includes
// those colors. Full config-row support is a TODO.
func EncodePresetFrame(p model.Preset) []byte {
	var out []byte

	// Row 0: preset header. Layout per CLAUDE.md / captures:
	// [0]=bank index, [1]=preset index, [2]=00, [3]=00.
	out = BuildRow(out, PresetRowHeader, []byte{byte(p.BankNum), byte(p.PresetNum), 0x00, 0x00})

	// Rows 1×32: one message row per slot.
	for i := range p.MsgArray {
		row := encodeMessageRow(p.MsgArray[i])
		out = BuildRow(out, PresetRowMessage, row)
	}

	// Row 2: shortName. Row 3: toggleName. Row 4: longName.
	// Row 5: config (emitted as zeros — TODO).
	// Row 6: shiftName.
	out = BuildRow(out, PresetRowShortName, encodeASCII(p.ShortName, nameRowLen))
	out = BuildRow(out, PresetRowToggleName, encodeASCII(p.ToggleName, nameRowLen))
	out = BuildRow(out, PresetRowLongName, encodeASCII(p.LongName, nameRowLen))
	out = BuildRow(out, PresetRowConfig, make([]byte, configRowLen))
	out = BuildRow(out, PresetRowShiftName, encodeASCII(p.ShiftName, nameRowLen))

	return out
}

// encodeMessageRow is the inverse of decodeMessageRow. See the comment
// on decodeMessageRow for the byte layout.
func encodeMessageRow(m model.Message) []byte {
	row := make([]byte, messageRowLen)
	row[0] = byte(m.M)
	row[1] = byte(m.Type)
	row[2] = byte(m.Data[0])
	row[3] = byte(m.Data[1])
	row[4] = byte(m.Data[2])
	row[5] = byte(m.Action)
	row[6] = byte(m.Channel)
	row[7] = byte(m.ToggleGroup)
	for i := 3; i < 18; i++ {
		row[5+i] = byte(m.Data[i])
	}
	return row
}

// encodeASCII pads/truncates a name string to the given length,
// space-padding on the right. This matches the wire format used by
// the MC8 Pro for all name fields.
func encodeASCII(s string, length int) []byte {
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = ' '
	}
	copy(buf, s)
	return buf
}
