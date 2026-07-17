package sysex

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

// controllerConfigLen is the fixed payload length for 03 21.
const controllerConfigLen = 32

// DecodeUUID decodes the payload of an 11 00 frame (controller UUID):
// 32 nibble-encoded wire bytes combined pairwise as (hi&0x0F)<<4 |
// lo&0x0F into 16 UUID bytes, rendered as a 32-char lowercase hex
// string. Matches the editor's ingest at editor.js:91240-91247.
//
// Note the editor DISPLAYS the UUID differently (Pro models: the
// first 7 bytes hyphen-separated with byte 6 repeated); see the
// State.UUID doc for the mapping.
func DecodeUUID(payload []byte) (string, error) {
	if len(payload) != 32 {
		return "", fmt.Errorf("sysex: uuid payload has %d bytes, want 32", len(payload))
	}
	raw := make([]byte, 16)
	for i := range raw {
		raw[i] = (payload[2*i]&0x0F)<<4 | payload[2*i+1]&0x0F
	}
	return hex.EncodeToString(raw), nil
}

// DecodeControllerConfig decodes the 32-byte payload of an 03 21
// frame into a [model.ControllerConfig].
func DecodeControllerConfig(payload []byte) (model.ControllerConfig, error) {
	if len(payload) < controllerConfigLen {
		return model.ControllerConfig{}, fmt.Errorf("sysex: controller config payload has %d bytes, want %d", len(payload), controllerConfigLen)
	}
	d := payload
	return model.ControllerConfig{
		MidiClockPersist:      d[0] != 0,
		DualLock:              d[1] != 0,
		SwitchSensitivity:     int(d[2]),
		MidiChannel:           int(d[3]),
		LcdAlign:              d[4] != 0,
		BankChangeDelayTime:   int(d[5]),
		BankChangeDisplayTime: int(d[6]),
		MidiThruUSBHost:       d[7] != 0,
		MidiThruUSBDevice:     d[8] != 0,
		MidiThruDIN5:          d[9] != 0,
		MidiThru35mm:          d[10] != 0,
		MidiThruBluetooth:     d[11] != 0,
		SavePresetToggle:      d[12] != 0,
		LongPressTime:         int(d[13]),
		BluetoothStartupDelay: int(d[14]),
		IgnoreMidiClock:       d[15] != 0,
		LoadLastBankOnStartup: d[16] != 0,
		NumMidiCable:          int(d[17]),
		MidiSendDelay:         int(d[18]),
		PresetMaxFontSize:     int(d[19]),
		ShowPresetLabels:      d[20] != 0,
		ScreenSaverTime:       int(d[21]),
		MidiClockOutputLSB:    int(d[22]),
		MidiClockOutputMSB:    int(d[23]),
		RelayPortA:            int(d[24]),
		RelayPortB:            int(d[25]),
		BrightnessValue:       int(d[26]),
		MiddleLayerFontSize:   int(d[27]),
		BankPageFontSize:      int(d[28]),
	}, nil
}

// EncodeControllerConfig produces the 32-byte payload for the 04 02
// controller-settings upload (same layout as the 03 21 dump).
func EncodeControllerConfig(c model.ControllerConfig) []byte {
	d := make([]byte, controllerConfigLen)
	boolByte := func(b bool) byte {
		if b {
			return 1
		}
		return 0
	}
	d[0] = boolByte(c.MidiClockPersist)
	d[1] = boolByte(c.DualLock)
	d[2] = byte(c.SwitchSensitivity)
	d[3] = byte(c.MidiChannel)
	d[4] = boolByte(c.LcdAlign)
	d[5] = byte(c.BankChangeDelayTime)
	d[6] = byte(c.BankChangeDisplayTime)
	d[7] = boolByte(c.MidiThruUSBHost)
	d[8] = boolByte(c.MidiThruUSBDevice)
	d[9] = boolByte(c.MidiThruDIN5)
	d[10] = boolByte(c.MidiThru35mm)
	d[11] = boolByte(c.MidiThruBluetooth)
	d[12] = boolByte(c.SavePresetToggle)
	d[13] = byte(c.LongPressTime)
	d[14] = byte(c.BluetoothStartupDelay)
	d[15] = boolByte(c.IgnoreMidiClock)
	d[16] = boolByte(c.LoadLastBankOnStartup)
	d[17] = byte(c.NumMidiCable)
	d[18] = byte(c.MidiSendDelay)
	d[19] = byte(c.PresetMaxFontSize)
	d[20] = boolByte(c.ShowPresetLabels)
	d[21] = byte(c.ScreenSaverTime)
	d[22] = byte(c.MidiClockOutputLSB)
	d[23] = byte(c.MidiClockOutputMSB)
	d[24] = byte(c.RelayPortA)
	d[25] = byte(c.RelayPortB)
	d[26] = byte(c.BrightnessValue)
	d[27] = byte(c.MiddleLayerFontSize)
	d[28] = byte(c.BankPageFontSize)
	return d
}

// DecodeWaveformEngines decodes the payload of a 03 24 frame
// (waveform engines). Layout, from the editor's buildFromArray
// (editor.js:13163-13174): [0]=count, then count × 4-byte entries
// [engineNum, min, max, type].
func DecodeWaveformEngines(payload []byte) ([]model.WaveformEngine, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: waveform engines payload empty")
	}
	n := int(payload[0])
	if len(payload) < 1+n*4 {
		return nil, fmt.Errorf("sysex: waveform engines payload too short for %d engines", n)
	}
	engines := make([]model.WaveformEngine, n)
	for i := 0; i < n; i++ {
		off := 1 + i*4
		engines[i] = model.WaveformEngine{
			EngineNum: int(payload[off]),
			Min:       int(payload[off+1]),
			Max:       int(payload[off+2]),
			Type:      int(payload[off+3]),
		}
	}
	return engines, nil
}

// EncodeWaveformEngines produces the WRITE payload for the 04 05
// upload (sendControllerWaveformData at editor.js:15571-15584):
// [count, then per engine min, max, type]. Note the asymmetry with
// DecodeWaveformEngines: the dump carries the engine number in each
// entry; the write omits it and relies on array order.
func EncodeWaveformEngines(engines []model.WaveformEngine) []byte {
	out := make([]byte, 1+len(engines)*3)
	out[0] = byte(len(engines))
	for i, e := range engines {
		off := 1 + i*3
		out[off] = byte(e.Min)
		out[off+1] = byte(e.Max)
		out[off+2] = byte(e.Type)
	}
	return out
}

// DecodeScrollCounters decodes the payload of a 03 26 frame (scroll
// counters). Layout, from the editor's buildFromArray
// (editor.js:12199-12211): [0]&0x1F=count, then count × 3-byte
// entries [min, max, startAt]. The editor sanitises inverted ranges
// (max <= min becomes 0..127); the SDK preserves the wire values
// instead so a read→write cycle is byte-faithful.
func DecodeScrollCounters(payload []byte) ([]model.ScrollCounter, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: scroll counters payload empty")
	}
	n := int(payload[0] & 0x1F)
	if len(payload) < 1+n*3 {
		return nil, fmt.Errorf("sysex: scroll counters payload too short for %d slots", n)
	}
	counters := make([]model.ScrollCounter, n)
	for i := 0; i < n; i++ {
		off := 1 + i*3
		counters[i] = model.ScrollCounter{
			Min:   int(payload[off]),
			Max:   int(payload[off+1]),
			Start: int(payload[off+2]),
		}
	}
	return counters, nil
}

// EncodeScrollCounters produces the WRITE payload for the 04 07
// upload (sendSlotCounterData at editor.js:15605-15612), which is the
// same layout as the dump: [count, then per slot min, max, startAt].
func EncodeScrollCounters(counters []model.ScrollCounter) []byte {
	out := make([]byte, 1+len(counters)*3)
	out[0] = byte(len(counters))
	for i, c := range counters {
		off := 1 + i*3
		out[off] = byte(c.Min)
		out[off+1] = byte(c.Max)
		out[off+2] = byte(c.Start)
	}
	return out
}

// DecodeResistorLadder decodes the payload of a 03 28 frame (aux
// switch calibration). Layout, from the editor's buildFromArray
// (editor.js:13933-13951): [0]=count, then count × 4-byte entries
// [switchNumber, triggerValue, f1, f2].
//
// The MC8 Pro's dump of this frame is one byte SHORT: the payload is
// 64 bytes but 16 switches need 65, so the final switch's f2 byte is
// missing. The editor reads undefined there and masks it to 0; we
// treat any bytes missing from the final entry as 0 the same way.
func DecodeResistorLadder(payload []byte) ([]model.ResistorLadderSwitch, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: resistor ladder payload empty")
	}
	n := int(payload[0])
	// Tolerate a truncated final entry (device firmware quirk) but
	// not a payload short by more than one entry's worth.
	if len(payload) < 1+(n-1)*4 {
		return nil, fmt.Errorf("sysex: resistor ladder payload too short for %d switches", n)
	}
	at := func(i int) byte {
		if i < len(payload) {
			return payload[i]
		}
		return 0
	}
	switches := make([]model.ResistorLadderSwitch, n)
	for i := 0; i < n; i++ {
		off := 1 + i*4
		switches[i] = model.ResistorLadderSwitch{
			SwitchNumber: int(at(off)),
			TriggerValue: int(at(off + 1)),
			F1:           int(at(off + 2)),
			F2:           int(at(off + 3)),
		}
	}
	return switches, nil
}

// EncodeResistorLadder produces the WRITE payload for the 04 0B
// upload (sendResistorLadderAuxSwitchData at editor.js:15680-15685,
// sending the manager's toArray): [count, then per switch
// switchNumber, triggerValue, f1, f2]. No padding — the editor sends
// exactly 1+4n bytes.
func EncodeResistorLadder(switches []model.ResistorLadderSwitch) []byte {
	out := make([]byte, 1+len(switches)*4)
	out[0] = byte(len(switches))
	for i, s := range switches {
		off := 1 + i*4
		out[off] = byte(s.SwitchNumber)
		out[off+1] = byte(s.TriggerValue)
		out[off+2] = byte(s.F1)
		out[off+3] = byte(s.F2)
	}
	return out
}

// DecodeMidiClockSlots decodes the payload of a 03 29 frame (MIDI
// clock BPM presets). Layout, from the editor's buildFromArray
// (editor.js:14062-14077):
//
//	[0]      reserved (skipped)
//	[1]      slot count, low 5 bits (max 16)
//	[2+2i]   BPM LSB for slot i (7-bit)
//	[3+2i]   BPM MSB for slot i (7-bit); BPM = msb<<7 | lsb
//
// Previously mis-assigned to the 03 28 frame with a guessed 4-byte
// entry structure; the editor's dispatch (case 41 = 0x29,
// editor.js:91148) settles both the frame and the layout.
func DecodeMidiClockSlots(payload []byte) ([]model.MidiClockSlot, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("sysex: midi clock slots payload too short (%d bytes)", len(payload))
	}
	n := int(payload[1] & 0x1F)
	if 2+2*n > len(payload) {
		return nil, fmt.Errorf("sysex: midi clock slots payload declares %d slots but has only %d bytes", n, len(payload))
	}
	slots := make([]model.MidiClockSlot, n)
	for i := 0; i < n; i++ {
		off := 2 + i*2
		lsb := int(payload[off] & 0x7F)
		msb := int(payload[off+1] & 0x7F)
		slots[i] = model.MidiClockSlot{BPM: msb<<7 | lsb}
	}
	return slots, nil
}

// EncodeMidiClockSlots produces the WRITE payload for the 04 0C
// upload (sendMidiClockSlotsData at editor.js:15686-15691, which
// sends the manager's getArray() output):
//
//	[0]      slot count
//	[1+2i]   BPM LSB for slot i
//	[2+2i]   BPM MSB for slot i
//
// BPM values are clamped to 0..500, matching the editor's slot class
// (editor.js:13967 `i > 500 && (i = 500)`) — without the clamp,
// BPM≥16384 would silently wrap the two 7-bit bytes. Slot-count
// validation lives in [Client.WriteMidiClockSlots]; the device holds
// exactly 16 slots and masks the count byte with &0x1F.
//
// Note the asymmetry with DecodeMidiClockSlots: the dump has a
// leading reserved byte before the count; the write does not.
func EncodeMidiClockSlots(slots []model.MidiClockSlot) []byte {
	out := make([]byte, 1+2*len(slots))
	out[0] = byte(len(slots))
	for i, s := range slots {
		bpm := s.BPM
		if bpm < 0 {
			bpm = 0
		}
		if bpm > 500 {
			bpm = 500
		}
		out[1+2*i] = byte(bpm & 0x7F)
		out[2+2*i] = byte(bpm >> 7 & 0x7F)
	}
	return out
}

// DecodeBankArrangement decodes the payload of a 03 22 frame (bank
// arrangement). Layout, from the editor's readFromArray
// (editor.js:12668-12681):
//
//	[0]      isActive
//	[1]      count field: 0 = "scan all slots", else count-1
//	[2..9]   reserved
//	[10..]   bank order table, one byte per slot (128 on the MC8
//	         Pro), 127 = unused slot
//
// NumBanksUsed follows the editor's read semantics: with a zero
// count field it is the number of non-127 entries; otherwise it is
// count field + 1.
func DecodeBankArrangement(payload []byte) (model.BankArrangement, error) {
	if len(payload) < 10+len(model.BankArrangement{}.BankOrder) {
		return model.BankArrangement{}, fmt.Errorf("sysex: bank arrangement payload too short (%d bytes)", len(payload))
	}
	var ba model.BankArrangement
	ba.IsActive = payload[0] != 0
	for i := range ba.BankOrder {
		ba.BankOrder[i] = int(payload[10+i])
	}
	if count := int(payload[1]); count != 0 {
		ba.NumBanksUsed = count + 1
	} else {
		for _, b := range ba.BankOrder {
			if b != 127 {
				ba.NumBanksUsed++
			}
		}
	}
	return ba, nil
}

// EncodeBankArrangement produces the WRITE payload for the 04 04
// upload (sendControllerBankArrangementData at editor.js:15654-15666):
// [isActive, activeCount-1, 8 reserved zero bytes, then the full
// 128-slot bank order table with 127 for unused slots]. An empty
// active list encodes count-1 as 0 rather than replicating the
// editor's underflow to 127.
func EncodeBankArrangement(ba model.BankArrangement) []byte {
	out := make([]byte, 10+len(ba.BankOrder))
	if ba.IsActive {
		out[0] = 1
	}
	if ba.NumBanksUsed > 0 {
		out[1] = byte(ba.NumBanksUsed - 1)
	}
	for i, b := range ba.BankOrder {
		out[10+i] = byte(b)
	}
	return out
}

// DecodeSequencerEngines decodes the payload of a 03 25 frame
// (sequencer engines). Layout, from the editor's buildFromArray
// (editor.js:13281-13293): [0]=count, then count × 18-byte entries
// [len&0x0F, reserved, arr[0..15]]. The engine number is not on the
// wire — it is the entry's position.
func DecodeSequencerEngines(payload []byte) ([]model.SequencerEngine, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: sequencer engines payload empty")
	}
	n := int(payload[0])
	if len(payload) < 1+n*18 {
		return nil, fmt.Errorf("sysex: sequencer engines payload too short for %d engines", n)
	}
	engines := make([]model.SequencerEngine, n)
	for i := 0; i < n; i++ {
		off := 1 + i*18
		engines[i].EngineNum = i
		engines[i].Len = int(payload[off] & 0x0F)
		for j := 0; j < 16; j++ {
			engines[i].Arr[j] = int(payload[off+2+j])
		}
	}
	return engines, nil
}

// EncodeSequencerEngines produces the WRITE payload for the 04 06
// upload (sendControllerSequencerData at editor.js:15588-15601):
// [count, then per engine len&0x0F, 0, arr[0..15]] — the same
// 18-byte entry layout as the dump.
func EncodeSequencerEngines(engines []model.SequencerEngine) []byte {
	out := make([]byte, 1+len(engines)*18)
	out[0] = byte(len(engines))
	for i, e := range engines {
		off := 1 + i*18
		out[off] = byte(e.Len & 0x0F)
		for j := 0; j < 16; j++ {
			out[off+2+j] = byte(e.Arr[j])
		}
	}
	return out
}

// omniportEntryLen is the wire size of one omniport entry. Dump and
// write directions share the same field order (the editor's
// buildFromArray at editor.js:12861-12881 and toArray at
// editor.js:12780-12782).
const omniportEntryLen = 11

// DecodeOmniports decodes the payload of a 03 23 frame (omniport /
// expression inputs). Layout: [0]=count, then count × 11-byte entries
// [portNum, type, fixedSwTip, td1, td2, fixedSwRing, rd1, rd2,
// fixedSwTipRing, trd1, trd2].
func DecodeOmniports(payload []byte) ([]model.OmniportInput, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: omniports payload empty")
	}
	n := int(payload[0])
	if len(payload) < 1+n*omniportEntryLen {
		return nil, fmt.Errorf("sysex: omniports payload too short for %d ports", n)
	}
	ports := make([]model.OmniportInput, n)
	for i := 0; i < n; i++ {
		off := 1 + i*omniportEntryLen
		ports[i] = model.OmniportInput{
			PortNum:        int(payload[off]),
			Type:           int(payload[off+1]),
			FixedSwTip:     int(payload[off+2]),
			TipData1:       int(payload[off+3]),
			TipData2:       int(payload[off+4]),
			FixedSwRing:    int(payload[off+5]),
			RingData1:      int(payload[off+6]),
			RingData2:      int(payload[off+7]),
			FixedSwTipRing: int(payload[off+8]),
			TipRingData1:   int(payload[off+9]),
			TipRingData2:   int(payload[off+10]),
		}
	}
	return ports, nil
}

// EncodeOmniports produces the WRITE payload for the 04 08 upload
// (sendControllerOmniPortData at editor.js:15673-15679, sending the
// manager's toArray). Note the asymmetry with DecodeOmniports: the
// dump has a leading count byte; the write is the bare concatenation
// of 11-byte port entries in the same field order.
func EncodeOmniports(ports []model.OmniportInput) []byte {
	out := make([]byte, 0, len(ports)*omniportEntryLen)
	for _, p := range ports {
		out = append(out,
			byte(p.PortNum), byte(p.Type),
			byte(p.FixedSwTip), byte(p.TipData1), byte(p.TipData2),
			byte(p.FixedSwRing), byte(p.RingData1), byte(p.RingData2),
			byte(p.FixedSwTipRing), byte(p.TipRingData1), byte(p.TipRingData2))
	}
	return out
}

// midiEventSlots is the fixed number of event processor rules
// (editor.js:12994 numEvents = 16).
const midiEventSlots = 16

// DecodeMidiEvents decodes the payload of a 03 27 frame (MIDI event
// processor). Layout, from the editor's readFromArray
// (editor.js:13028-13051): 16 rows of
// [0x7F, slotIndex, rowLen, numberFrom, numberTo, channelFrom,
// channelTo, typeFrom, typeTo, valueFrom, valueTo,
// toSetOutgoingValue, toMapInputOutput, toMapValue].
func DecodeMidiEvents(payload []byte) ([midiEventSlots]model.MidiEvent, error) {
	var events [midiEventSlots]model.MidiEvent
	for i := 0; i+14 <= len(payload); {
		if payload[i] != 0x7F {
			i++
			continue
		}
		slot := int(payload[i+1])
		if slot >= midiEventSlots {
			return events, fmt.Errorf("sysex: midi event row has out-of-range slot %d", slot)
		}
		d := payload[i+3 : i+14]
		events[slot] = model.MidiEvent{
			NumberFrom:         int(d[0]),
			NumberTo:           int(d[1]),
			ChannelFrom:        int(d[2]),
			ChannelTo:          int(d[3]),
			TypeFrom:           int(d[4]),
			TypeTo:             int(d[5]),
			ValueFrom:          int(d[6]),
			ValueTo:            int(d[7]),
			ToSetOutgoingValue: d[8] != 0,
			ToMapInputOutput:   int(d[9] & 0x03),
			ToMapValue:         d[10] != 0,
		}
		i += 14
	}
	return events, nil
}

// EncodeMidiEvents produces the WRITE payload for the 04 0A upload
// (getArray at editor.js:13052-13072) — the same 14-byte row layout
// as the dump, always all 16 slots.
func EncodeMidiEvents(events [midiEventSlots]model.MidiEvent) []byte {
	boolByte := func(b bool) byte {
		if b {
			return 1
		}
		return 0
	}
	out := make([]byte, 0, midiEventSlots*14)
	for i, e := range events {
		out = append(out, 0x7F, byte(i), 11,
			byte(e.NumberFrom), byte(e.NumberTo),
			byte(e.ChannelFrom), byte(e.ChannelTo),
			byte(e.TypeFrom), byte(e.TypeTo),
			byte(e.ValueFrom), byte(e.ValueTo),
			boolByte(e.ToSetOutgoingValue),
			byte(e.ToMapInputOutput&0x03),
			boolByte(e.ToMapValue))
	}
	return out
}

// DecodeMidiChannels decodes the payload of a 03 20 frame.
// Three row-tag ranges: 0x00–0x0F = 16-byte channel names,
// 0x10–0x1F = 4-byte remap/port config, 0x20–0x2F = 16-byte attributes.
func DecodeMidiChannels(payload []byte) ([16]model.MidiChannel, error) {
	rows, err := ParseRows(payload)
	if err != nil {
		return [16]model.MidiChannel{}, err
	}
	var channels [16]model.MidiChannel
	for _, row := range rows {
		tag := int(row.Tag)
		switch {
		case tag < 16:
			channels[tag].Name = strings.TrimRight(string(row.Data), " ")
		case tag < 32:
			ch := tag - 16
			if len(row.Data) >= 4 {
				channels[ch].Remap = int(row.Data[0])
				channels[ch].PortMSB = int(row.Data[1])
				channels[ch].PortLSB = int(row.Data[2])
			}
		case tag < 48:
			ch := tag - 32
			for j := 0; j < len(row.Data) && j < 16; j++ {
				channels[ch].Attributes[j] = int(row.Data[j])
			}
		}
	}
	return channels, nil
}

// EncodeMidiChannels produces the payload for a 03 20 frame.
func EncodeMidiChannels(channels [16]model.MidiChannel) []byte {
	var out []byte
	// Row tags 0–15: 16-byte ASCII names.
	for i, ch := range channels {
		name := make([]byte, 16)
		for j := range name {
			name[j] = ' '
		}
		copy(name, ch.Name)
		out = BuildRow(out, byte(i), name)
	}
	// Row tags 16–31: 4-byte remap config.
	for i, ch := range channels {
		data := []byte{byte(ch.Remap), byte(ch.PortMSB), byte(ch.PortLSB), 0}
		out = BuildRow(out, byte(16+i), data)
	}
	// Row tags 32–47: 16-byte attributes.
	for i, ch := range channels {
		data := make([]byte, 16)
		for j := 0; j < 16; j++ {
			data[j] = byte(ch.Attributes[j])
		}
		out = BuildRow(out, byte(32+i), data)
	}
	return out
}
