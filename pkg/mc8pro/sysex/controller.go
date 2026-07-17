package sysex

import (
	"fmt"
	"strings"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

// controllerConfigLen is the fixed payload length for 11 00.
const controllerConfigLen = 32

// DecodeControllerConfig decodes the 32-byte payload of an 11 00
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

// EncodeControllerConfig produces the 32-byte payload for an 11 00 frame.
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

// DecodeWaveformEngines decodes the payload of a 03 26 frame.
// Layout: [0]=count, then count × 3-byte entries [type, max, min].
func DecodeWaveformEngines(payload []byte) ([]model.WaveformEngine, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: waveform engines payload empty")
	}
	n := int(payload[0])
	if len(payload) < 1+n*3 {
		return nil, fmt.Errorf("sysex: waveform engines payload too short for %d engines", n)
	}
	engines := make([]model.WaveformEngine, n)
	for i := 0; i < n; i++ {
		off := 1 + i*3
		engines[i] = model.WaveformEngine{
			Min:  int(payload[off]),
			Max:  int(payload[off+1]),
			Type: int(payload[off+2]),
		}
	}
	return engines, nil
}

// EncodeWaveformEngines produces the payload for a 03 26 frame.
// Byte order matches editor.js: [min, max, type] per engine.
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

// DecodeResistorLadder decodes the payload of a 03 25 frame.
// Layout: [0]=count, then count × 4-byte entries, rest is padding.
func DecodeResistorLadder(payload []byte) ([]model.ResistorLadderSwitch, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: resistor ladder payload empty")
	}
	n := int(payload[0])
	if len(payload) < 1+n*4 {
		return nil, fmt.Errorf("sysex: resistor ladder payload too short for %d switches", n)
	}
	switches := make([]model.ResistorLadderSwitch, n)
	for i := 0; i < n; i++ {
		off := 1 + i*4
		switches[i] = model.ResistorLadderSwitch{
			SwitchNumber: int(payload[off]),
			TriggerValue: int(payload[off+1]),
			F1:           int(payload[off+2]),
			F2:           int(payload[off+3]),
		}
	}
	return switches, nil
}

// EncodeResistorLadder produces the payload for a 03 25 frame.
// Pads to 145 bytes to match observed wire length.
func EncodeResistorLadder(switches []model.ResistorLadderSwitch) []byte {
	dataLen := 1 + len(switches)*4
	padLen := 145
	if dataLen > padLen {
		padLen = dataLen
	}
	out := make([]byte, padLen)
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

// DecodeMidiClockSlots decodes the payload of a 03 28 frame.
// Layout: [0]=count, then count × 4-byte entries starting at [1]:
// [index, bpm_lsb, bpm_msb, reserved]. Last entry may be 3 bytes
// (no trailing pad) if count×4+1 > payload length.
func DecodeMidiClockSlots(payload []byte) ([]model.MidiClockSlot, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: midi clock slots payload empty")
	}
	n := int(payload[0])
	slots := make([]model.MidiClockSlot, n)
	for i := 0; i < n; i++ {
		off := 1 + i*4
		if off+2 >= len(payload) {
			break
		}
		// [off]=index, [off+1]=bpm_lsb, [off+2]=bpm_msb
		lsb := int(payload[off+1])
		msb := int(payload[off+2])
		slots[i] = model.MidiClockSlot{BPM: (msb << 7) | lsb}
	}
	return slots, nil
}

// EncodeMidiClockSlots produces the payload for a 03 28 frame.
// Pads to 64 bytes to match observed wire length.
func EncodeMidiClockSlots(slots []model.MidiClockSlot) []byte {
	padLen := 64
	out := make([]byte, padLen)
	out[0] = byte(len(slots))
	for i, s := range slots {
		off := 1 + i*4
		if off+2 >= padLen {
			break
		}
		out[off] = byte(i)                       // index
		out[off+1] = byte(s.BPM & 0x7F)          // bpm LSB
		out[off+2] = byte((s.BPM >> 7) & 0x7F)   // bpm MSB
		// [off+3] = reserved (0)
	}
	return out
}

// DecodeBankArrangement decodes the 50-byte payload of a 03 21 frame.
func DecodeBankArrangement(payload []byte) (model.BankArrangement, error) {
	if len(payload) < 40 {
		return model.BankArrangement{}, fmt.Errorf("sysex: bank arrangement payload too short (%d bytes)", len(payload))
	}
	var ba model.BankArrangement
	ba.IsActive = payload[0] != 0
	ba.NumBanksUsed = int(payload[1]) + 1
	for i := 0; i < 30; i++ {
		ba.BankOrder[i] = int(payload[10+i])
	}
	return ba, nil
}

// EncodeBankArrangement produces the 50-byte payload for a 03 21 frame.
func EncodeBankArrangement(ba model.BankArrangement) []byte {
	out := make([]byte, 50)
	if ba.IsActive {
		out[0] = 1
	}
	n := ba.NumBanksUsed
	if n > 0 {
		n--
	}
	out[1] = byte(n)
	for i := 0; i < 30; i++ {
		out[10+i] = byte(ba.BankOrder[i])
	}
	return out
}

// DecodeSequencerEngines decodes the payload of a 03 23 frame.
// Layout: [0]=count, then count × 11-byte entries [engineNum, len, arr[9]].
// Trailing bytes are padding.
func DecodeSequencerEngines(payload []byte) ([]model.SequencerEngine, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: sequencer engines payload empty")
	}
	n := int(payload[0])
	if len(payload) < 1+n*11 {
		return nil, fmt.Errorf("sysex: sequencer engines payload too short for %d engines", n)
	}
	engines := make([]model.SequencerEngine, n)
	for i := 0; i < n; i++ {
		off := 1 + i*11
		engines[i].EngineNum = int(payload[off])
		engines[i].Len = int(payload[off+1])
		for j := 0; j < 9; j++ {
			engines[i].Arr[j] = int(payload[off+2+j])
		}
	}
	return engines, nil
}

// EncodeSequencerEngines produces the payload for a 03 23 frame.
// Pads to 64 bytes to match observed wire length.
func EncodeSequencerEngines(engines []model.SequencerEngine) []byte {
	dataLen := 1 + len(engines)*11
	padLen := 64
	if dataLen > padLen {
		padLen = dataLen
	}
	out := make([]byte, padLen)
	out[0] = byte(len(engines))
	for i, e := range engines {
		off := 1 + i*11
		out[off] = byte(e.EngineNum)
		out[off+1] = byte(e.Len)
		for j := 0; j < 9; j++ {
			out[off+2+j] = byte(e.Arr[j])
		}
	}
	return out
}

// DecodeOmniports decodes the payload of a 03 24 frame.
// Layout: [0]=count, then count × 4-byte entries [portNum, type, val1, val2].
func DecodeOmniports(payload []byte) ([]model.OmniportInput, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("sysex: omniports payload empty")
	}
	n := int(payload[0])
	if len(payload) < 1+n*4 {
		return nil, fmt.Errorf("sysex: omniports payload too short for %d ports", n)
	}
	ports := make([]model.OmniportInput, n)
	for i := 0; i < n; i++ {
		off := 1 + i*4
		ports[i] = model.OmniportInput{
			PortNum: int(payload[off]),
			Type:    int(payload[off+1]),
			Val1:    int(payload[off+2]),
			Val2:    int(payload[off+3]),
		}
	}
	return ports, nil
}

// EncodeOmniports produces the payload for a 03 24 frame.
func EncodeOmniports(ports []model.OmniportInput) []byte {
	out := make([]byte, 1+len(ports)*4)
	out[0] = byte(len(ports))
	for i, p := range ports {
		off := 1 + i*4
		out[off] = byte(p.PortNum)
		out[off+1] = byte(p.Type)
		out[off+2] = byte(p.Val1)
		out[off+3] = byte(p.Val2)
	}
	return out
}

// DecodeMidiEvents decodes the payload of a 03 22 frame.
// Layout: 10-byte header + 128-byte remap table (byte[i] = output for input i).
func DecodeMidiEvents(payload []byte) (model.MidiEventProcessor, error) {
	if len(payload) < 138 {
		return model.MidiEventProcessor{}, fmt.Errorf("sysex: midi events payload has %d bytes, want 138", len(payload))
	}
	var ep model.MidiEventProcessor
	copy(ep.Header[:], payload[:10])
	copy(ep.RemapTable[:], payload[10:138])
	return ep, nil
}

// EncodeMidiEvents produces the payload for a 03 22 frame.
func EncodeMidiEvents(ep model.MidiEventProcessor) []byte {
	out := make([]byte, 138)
	copy(out[:10], ep.Header[:])
	copy(out[10:], ep.RemapTable[:])
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
