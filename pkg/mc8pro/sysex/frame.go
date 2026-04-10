// Package sysex implements the Morningstar MC8 Pro SysEx wire format.
//
// Frame layout (see ../../CLAUDE.md for the derivation):
//
//	[0]     0xF0              SysEx start
//	[1..3]  00 21 24           Morningstar manufacturer ID
//	[4]     DEVICE_MODEL_ID   (8 = MC8 Pro)
//	[5]     0                 outgoing; device replies set this to 4
//	[6..7]  CMD1 CMD2         command selector
//	[8..11] ARG1..ARG4        command-specific arguments
//	[12..13] 0                reserved
//	[14..15] LEN_MSB LEN_LSB  payload length (7-bit pair), 0 for requests
//	[16..n-3] payload         variable
//	[n-2]   CHECKSUM          XOR of bytes[0..n-3] masked to 7 bits
//	[n-1]   0xF7              SysEx end
//
// All data bytes (non-header, non-framing) must be ≤ 0x7F.
package sysex

import "errors"

// Byte constants used throughout the Morningstar frame layout.
const (
	SysExStart      = 0xF0
	SysExEnd        = 0xF7
	ManfID1         = 0x00
	ManfID2         = 0x21
	ManfID3         = 0x24
	ModelMC8Pro     = 0x08
	VersionOutgoing = 0x00 // byte 5 in requests from editor→device
	VersionIncoming = 0x04 // byte 5 in replies from device→editor
)

// Header field offsets. These are the byte indices into a complete
// SysEx frame (including F0 and F7 terminators).
const (
	OffsetStart    = 0
	OffsetManf1    = 1
	OffsetManf2    = 2
	OffsetManf3    = 3
	OffsetModel    = 4
	OffsetVersion  = 5
	OffsetCmd1     = 6
	OffsetCmd2     = 7
	OffsetArg1     = 8
	OffsetArg2     = 9
	OffsetArg3     = 10
	OffsetArg4     = 11
	OffsetRes1     = 12
	OffsetRes2     = 13
	OffsetLenMSB   = 14
	OffsetLenLSB   = 15
	HeaderSize     = 16 // bytes [0..15], payload starts at [16]
	TrailerSize    = 2  // checksum + 0xF7
	MinFrameSize   = HeaderSize + TrailerSize
)

// Frame is a decoded representation of a Morningstar SysEx message.
// The Payload slice is a view into the frame body after the 16-byte
// header and before the checksum+F7 trailer. It does NOT include the
// length bytes; those are recomputed on encode.
type Frame struct {
	Model    byte    // byte 4; should be ModelMC8Pro
	Version  byte    // byte 5; 0 outgoing, 4 incoming
	Cmd1     byte    // byte 6
	Cmd2     byte    // byte 7
	Args     [4]byte // bytes 8..11
	Payload  []byte  // bytes 16..n-3
	RawCksum byte    // the checksum byte from the wire (byte n-2); needed for ACK
}

// Errors returned by Parse.
var (
	ErrTooShort       = errors.New("sysex: frame too short")
	ErrBadStart       = errors.New("sysex: missing 0xF0 start byte")
	ErrBadEnd         = errors.New("sysex: missing 0xF7 end byte")
	ErrBadManufacturer = errors.New("sysex: manufacturer ID is not Morningstar (00 21 24)")
	ErrBadChecksum    = errors.New("sysex: checksum mismatch")
	ErrBadLength      = errors.New("sysex: length field does not match frame size")
)

// Checksum computes the Morningstar SysEx checksum over the given
// bytes. It XORs everything from index 0 up to and including index
// len(b)-3 (that is, everything except the checksum slot itself and
// the trailing 0xF7) and masks the result to 7 bits.
//
// This implementation matches editor.js:18806 calculateCheckSum.
// The input must be the FULL frame including 0xF0 ... 0xF7, with the
// checksum slot already present (value is ignored).
func Checksum(frame []byte) byte {
	if len(frame) < 3 {
		return 0
	}
	c := frame[0]
	for i := 1; i < len(frame)-2; i++ {
		c ^= frame[i]
	}
	return c & 0x7F
}

// Build constructs a complete SysEx frame for an outgoing editor→device
// message. Byte 5 (version) is set to 0 and bytes 12-15 (reserved +
// length) are left at zero — this matches what the editor sends
// (editor.js:18833 sendSysex6 never populates the length field on
// outgoing frames; it's only ever read on incoming device→editor
// responses). The checksum and trailing 0xF7 are appended.
//
// Payload bytes are copied verbatim; the caller is responsible for
// ensuring they are all ≤ 0x7F (MIDI SysEx data-byte constraint).
func Build(cmd1, cmd2 byte, args [4]byte, payload []byte) []byte {
	frame := make([]byte, HeaderSize, HeaderSize+len(payload)+TrailerSize)
	frame[OffsetStart] = SysExStart
	frame[OffsetManf1] = ManfID1
	frame[OffsetManf2] = ManfID2
	frame[OffsetManf3] = ManfID3
	frame[OffsetModel] = ModelMC8Pro
	frame[OffsetVersion] = VersionOutgoing
	frame[OffsetCmd1] = cmd1
	frame[OffsetCmd2] = cmd2
	frame[OffsetArg1] = args[0]
	frame[OffsetArg2] = args[1]
	frame[OffsetArg3] = args[2]
	frame[OffsetArg4] = args[3]
	// frame[12..15] remain 0
	frame = append(frame, payload...)
	frame = append(frame, 0x00, SysExEnd)
	frame[len(frame)-2] = Checksum(frame)
	return frame
}

// Parse decodes a SysEx frame into a [Frame] struct. It validates the
// start/end bytes, manufacturer ID, and checksum. It does NOT validate
// the length field against the frame size (some observed responses
// have length-field quirks we don't fully understand); instead it
// trusts the actual frame size as delivered by the MIDI driver.
func Parse(b []byte) (Frame, error) {
	var f Frame
	if len(b) < MinFrameSize {
		return f, ErrTooShort
	}
	if b[OffsetStart] != SysExStart {
		return f, ErrBadStart
	}
	if b[len(b)-1] != SysExEnd {
		return f, ErrBadEnd
	}
	if b[OffsetManf1] != ManfID1 || b[OffsetManf2] != ManfID2 || b[OffsetManf3] != ManfID3 {
		return f, ErrBadManufacturer
	}
	if got, want := b[len(b)-2], Checksum(b); got != want {
		return f, ErrBadChecksum
	}
	f.Model = b[OffsetModel]
	f.Version = b[OffsetVersion]
	f.Cmd1 = b[OffsetCmd1]
	f.Cmd2 = b[OffsetCmd2]
	f.Args[0] = b[OffsetArg1]
	f.Args[1] = b[OffsetArg2]
	f.Args[2] = b[OffsetArg3]
	f.Args[3] = b[OffsetArg4]
	// Payload is everything between the 16-byte header and the
	// 2-byte trailer. Return a sub-slice; callers that need ownership
	// should copy.
	f.Payload = b[HeaderSize : len(b)-TrailerSize]
	f.RawCksum = b[len(b)-2]
	return f, nil
}
