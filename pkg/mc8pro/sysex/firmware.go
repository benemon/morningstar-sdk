package sysex

import "errors"

// FirmwareInfo is the decoded contents of a firmware frame. It is
// intentionally a flat data type with no dependencies on mc8pro types,
// so the sysex package stays free of an import cycle. Callers in
// the parent mc8pro package convert this into the public mc8pro.Device
// type.
type FirmwareInfo struct {
	Model  byte
	Major  byte
	Minor  byte
	Patch  byte
	Serial [4]byte
}

// DecodeFirmwareFrame extracts firmware version and device serial from
// the device's reply to REQUEST_CONTROLLER_FIRMWARE_VERSION
// (cmd1=0x11, cmd2=0x03).
//
// The frame layout (verified empirically; see CLAUDE.md):
//
//	bytes 0..15  : standard 16-byte SysEx header. Args are at bytes 8..10:
//	               args[0] = major, args[1] = minor, args[2] = patch
//	bytes 16..n-3: payload made of repeating 7F-tag rows. We extract:
//	               row tag 0 → 1 byte: major (redundant, matches args[0])
//	               row tag 1 → 1 byte: minor
//	               row tag 2 → 1 byte: patch
//	               row tag 3 → 4 bytes: device serial
//
// We trust the payload rows over the args because the row tags are
// self-describing and survive any future header changes.
func DecodeFirmwareFrame(f Frame) (FirmwareInfo, error) {
	if f.Cmd1 != 0x11 || f.Cmd2 != 0x03 {
		return FirmwareInfo{}, errors.New("sysex: not a firmware frame (expected cmd 11 03)")
	}
	rows, err := ParseRows(f.Payload)
	if err != nil {
		return FirmwareInfo{}, err
	}

	info := FirmwareInfo{Model: f.Model}
	var sawSerial bool
	for _, row := range rows {
		switch row.Tag {
		case 0x00:
			if len(row.Data) >= 1 {
				info.Major = row.Data[0]
			}
		case 0x01:
			if len(row.Data) >= 1 {
				info.Minor = row.Data[0]
			}
		case 0x02:
			if len(row.Data) >= 1 {
				info.Patch = row.Data[0]
			}
		case 0x03:
			if len(row.Data) >= 4 {
				copy(info.Serial[:], row.Data[:4])
				sawSerial = true
			}
		}
	}
	if !sawSerial {
		return FirmwareInfo{}, errors.New("sysex: firmware frame missing serial row")
	}
	return info, nil
}
