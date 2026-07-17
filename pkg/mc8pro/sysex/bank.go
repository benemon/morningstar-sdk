package sysex

import (
	"fmt"
	"strings"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

// Row tags inside a bank metadata frame (Cmd1=0x06 or 0x07, Cmd2=0x02).
const (
	BankMetaRowBankNum = 0x00 // 1 byte: bank number
	BankMetaRowConfig  = 0x01 // 8 bytes (Pro): bank-level config flags
	BankMetaRowMessage = 0x02 // 14 bytes (Pro): one bank-level message
	BankMetaRowName    = 0x03 // 32 bytes (MC8 Pro): bank name ASCII
	BankMetaRowDescr   = 0x04 // 32 bytes (MC8 Pro, Pro only): bank description ASCII
)

// bankConfigRowLen is the length of the Pro bank config row (tag 1).
const bankConfigRowLen = 8

// bankMsgRowLen is the length of a Pro bank message row (tag 2).
const bankMsgRowLen = 14

// bankNameLen is the MC8 Pro bank name field length.
const bankNameLen = 32

// DecodeBankMetaFrame takes the payload of a bank metadata frame
// (command 06 02 or 07 02) and populates the bank-level fields of a
// [model.Bank]: config flags, BankMsgArray, BankName, and
// BankDescription. BankNumber is also set from row tag 0.
//
// Fields decoded from the config row (tag 1):
//   - BankClearToggle, ToDisplay, BackgroundColor, TextColor,
//     IsColorEnabled, PageLimit
//
// Bank messages (tag 2) use a 14-byte format for Pro models carrying
// data[0..8]. data[9..17] are left at zero since they are not
// present on the wire for bank-level messages.
func DecodeBankMetaFrame(payload []byte) (model.Bank, error) {
	rows, err := ParseRows(payload)
	if err != nil {
		return model.Bank{}, err
	}

	var b model.Bank

	for _, row := range rows {
		switch row.Tag {
		case BankMetaRowBankNum:
			if len(row.Data) < 1 {
				return model.Bank{}, fmt.Errorf("sysex: bank meta row 0 has no data")
			}
			b.BankNumber = int(row.Data[0])

		case BankMetaRowConfig:
			decodeBankConfigRow(row.Data, &b)

		case BankMetaRowMessage:
			msg, err := decodeBankMessageRow(row.Data)
			if err != nil {
				return model.Bank{}, err
			}
			if msg.M < 0 || msg.M >= len(b.BankMsgArray) {
				return model.Bank{}, fmt.Errorf("sysex: bank message slot index %d out of range", msg.M)
			}
			b.BankMsgArray[msg.M] = msg

		case BankMetaRowName:
			b.BankName = strings.TrimRight(string(row.Data), " ")

		case BankMetaRowDescr:
			b.BankDescription = strings.TrimRight(string(row.Data), " ")
		}
	}

	return b, nil
}

// decodeBankConfigRow populates bank-level config fields from the
// config row (tag 1). Handles two sizes:
//   - 2 bytes: standard models (bankClearToggle only)
//   - 8 bytes: Pro models (full config)
//
// See CLAUDE.md "Bank metadata row layout" for the byte map.
func decodeBankConfigRow(data []byte, b *model.Bank) {
	if len(data) < 1 {
		return
	}
	b.BankClearToggle = data[0] != 0

	if len(data) >= 8 {
		b.ToDisplay = data[1] != 0
		b.BackgroundColor = int(data[2])
		b.TextColor = int(data[3])
		b.IsColorEnabled = data[4] != 0
		b.PageLimit = int(data[5])
	}
}

// decodeBankMessageRow decodes a 14-byte (Pro) or 8-byte (standard)
// bank message row into a [model.Message].
//
// Pro layout (14 bytes):
//
//	[0]     m   (slot index 0..31)
//	[1]     t   (message type)
//	[2]     data[0]
//	[3]     data[1]
//	[4]     data[2]
//	[5]     c   (MIDI channel)
//	[6]     a   (action/trigger)
//	[7]     tg  (toggle group)
//	[8..13] data[3..8]
func decodeBankMessageRow(data []byte) (model.Message, error) {
	if len(data) < 8 {
		return model.Message{}, fmt.Errorf("sysex: bank message row has %d bytes, want at least 8", len(data))
	}
	var m model.Message
	m.M = int(data[0])
	m.Type = int(data[1])
	m.Data[0] = int(data[2])
	m.Data[1] = int(data[3])
	m.Data[2] = int(data[4])
	m.Channel = int(data[5])
	m.Action = int(data[6])
	m.Toggle = int(data[7])

	// Pro models carry data[3..8] in bytes [8..13].
	if len(data) >= bankMsgRowLen {
		for i := 0; i < 6; i++ {
			m.Data[3+i] = int(data[8+i])
		}
	}
	return m, nil
}

// EncodeBankMetaFrame produces the payload bytes for a bank metadata
// frame from a [model.Bank]. The inverse of DecodeBankMetaFrame.
// Emits the MC8 Pro format (8-byte config, 14-byte messages, 32-byte
// names).
func EncodeBankMetaFrame(b model.Bank) []byte {
	var out []byte

	// Row 0: bank number.
	out = BuildRow(out, BankMetaRowBankNum, []byte{byte(b.BankNumber)})

	// Row 1: bank config (8 bytes for Pro).
	out = BuildRow(out, BankMetaRowConfig, encodeBankConfigRow(b))

	// Row 2 × 32: bank messages.
	for i := range b.BankMsgArray {
		out = BuildRow(out, BankMetaRowMessage, encodeBankMessageRow(b.BankMsgArray[i]))
	}

	// Row 3: bank name.
	out = BuildRow(out, BankMetaRowName, encodeBankASCII(b.BankName, bankNameLen))

	// Row 4: bank description (Pro only).
	out = BuildRow(out, BankMetaRowDescr, encodeBankASCII(b.BankDescription, bankNameLen))

	return out
}

// encodeBankConfigRow produces the 8-byte Pro bank config row.
func encodeBankConfigRow(b model.Bank) []byte {
	buf := make([]byte, bankConfigRowLen)
	if b.BankClearToggle {
		buf[0] = 1
	}
	if b.ToDisplay {
		buf[1] = 1
	}
	buf[2] = byte(b.BackgroundColor)
	buf[3] = byte(b.TextColor)
	if b.IsColorEnabled {
		buf[4] = 1
	}
	buf[5] = byte(b.PageLimit)
	return buf
}

// encodeBankMessageRow produces a 14-byte Pro bank message row.
func encodeBankMessageRow(m model.Message) []byte {
	row := make([]byte, bankMsgRowLen)
	row[0] = byte(m.M)
	row[1] = byte(m.Type)
	row[2] = byte(m.Data[0])
	row[3] = byte(m.Data[1])
	row[4] = byte(m.Data[2])
	row[5] = byte(m.Channel)
	row[6] = byte(m.Action)
	row[7] = byte(m.Toggle)
	for i := 0; i < 6; i++ {
		row[8+i] = byte(m.Data[3+i])
	}
	return row
}

// encodeBankASCII pads/truncates a string to the given length with
// trailing spaces.
func encodeBankASCII(s string, length int) []byte {
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = ' '
	}
	copy(buf, s)
	return buf
}
