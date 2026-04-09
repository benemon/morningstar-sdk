package sysex

import "fmt"

// RowMarker is the byte that introduces a row inside a Morningstar
// payload. It's always 0x7F because that's the max 7-bit data value
// and never appears at the start of legitimate row content.
const RowMarker = 0x7F

// Row is one entry inside a framed payload. The Tag byte's meaning
// depends on the containing command:
//
//   - For per-preset frames (06 01, 07 01): Tag identifies the row
//     "kind" (0=bank header, 1=message, 2..6=preset metadata/names)
//   - For batched frames (11 05, 09 01, 03 20, 11 04): Tag identifies
//     the entry index within the batch (preset slot, bank index, etc)
//
// Data is a sub-slice into the original payload; callers that need to
// retain it beyond the lifetime of the payload must copy.
type Row struct {
	Tag  byte
	Data []byte
}

// ErrMalformedRow is returned by ParseRows when the byte stream doesn't
// match the expected 7F <tag> <len> <data[len]> pattern.
type ErrMalformedRow struct {
	Offset int
	Reason string
}

func (e *ErrMalformedRow) Error() string {
	return fmt.Sprintf("sysex: malformed row at payload offset %d: %s", e.Offset, e.Reason)
}

// ParseRows walks a frame payload and extracts all rows. The payload
// is the slice returned by [Frame.Payload] — that is, everything
// between the 16-byte header and the 2-byte trailer. Trailing bytes
// that don't form a complete row are ignored (some frames have a
// trailing zero byte after the last row).
func ParseRows(payload []byte) ([]Row, error) {
	var rows []Row
	i := 0
	for i < len(payload) {
		if payload[i] != RowMarker {
			// Some frames have a single trailing byte after the last
			// row. If only one byte remains we accept it silently;
			// otherwise we report it as malformed.
			if i == len(payload)-1 {
				return rows, nil
			}
			return nil, &ErrMalformedRow{
				Offset: i,
				Reason: fmt.Sprintf("expected row marker 0x7F, got 0x%02X", payload[i]),
			}
		}
		if i+3 > len(payload) {
			return nil, &ErrMalformedRow{
				Offset: i,
				Reason: "truncated row header (need marker + tag + length)",
			}
		}
		tag := payload[i+1]
		length := int(payload[i+2])
		dataStart := i + 3
		dataEnd := dataStart + length
		if dataEnd > len(payload) {
			return nil, &ErrMalformedRow{
				Offset: i,
				Reason: fmt.Sprintf("row tag=0x%02X claims %d bytes but only %d remain", tag, length, len(payload)-dataStart),
			}
		}
		rows = append(rows, Row{
			Tag:  tag,
			Data: payload[dataStart:dataEnd],
		})
		i = dataEnd
	}
	return rows, nil
}

// BuildRow appends a single row to dst in wire format and returns the
// extended slice. length is taken from len(data).
func BuildRow(dst []byte, tag byte, data []byte) []byte {
	if len(data) > 0x7F {
		panic(fmt.Sprintf("sysex: row data too long (%d bytes); max 127 bytes per row", len(data)))
	}
	dst = append(dst, RowMarker, tag, byte(len(data)))
	dst = append(dst, data...)
	return dst
}
