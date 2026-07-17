package model

// ControllerConfig holds the global device settings decoded from the
// 03 21 frame. These are device-wide settings that apply regardless
// of which bank is active.
type ControllerConfig struct {
	MidiClockPersist      bool `json:"midiClockPersist"`
	DualLock              bool `json:"dualLock"`
	SwitchSensitivity     int  `json:"switchSensitivity"` // 0–4
	MidiChannel           int  `json:"midiChannel"`       // 1–16
	LcdAlign              bool `json:"lcdAlign"`
	BankChangeDelayTime   int  `json:"bankChangeDelayTime"`
	BankChangeDisplayTime int  `json:"bankChangeDisplayTime"`
	MidiThruUSBHost       bool `json:"midiThruUsbHost"`   // Pro only
	MidiThruUSBDevice     bool `json:"midiThruUsbDevice"` // Pro only
	MidiThruDIN5          bool `json:"midiThruDin5"`      // Pro only
	MidiThru35mm          bool `json:"midiThru35mm"`      // Pro only
	MidiThruBluetooth     bool `json:"midiThruBluetooth"` // Pro only
	SavePresetToggle      bool `json:"savePresetToggle"`
	LongPressTime         int  `json:"longPressTime"`         // tenths of second
	BluetoothStartupDelay int  `json:"bluetoothStartupDelay"` // Pro only
	IgnoreMidiClock       bool `json:"ignoreMidiClock"`
	LoadLastBankOnStartup bool `json:"loadLastBankOnStartup"`
	NumMidiCable          int  `json:"numMidiCable"`
	MidiSendDelay         int  `json:"midiSendDelay"`
	PresetMaxFontSize     int  `json:"presetMaxFontSize"`
	ShowPresetLabels      bool `json:"showPresetLabels"`
	ScreenSaverTime       int  `json:"screenSaverTime"`
	MidiClockOutputLSB    int  `json:"midiClockOutputLsb"` // 7-bit port bitmap low
	MidiClockOutputMSB    int  `json:"midiClockOutputMsb"` // 7-bit port bitmap high
	RelayPortA            int  `json:"relayPortA"`
	RelayPortB            int  `json:"relayPortB"`
	BrightnessValue       int  `json:"brightnessValue"` // 0–7, Pro only
	MiddleLayerFontSize   int  `json:"middleLayerFontSize"`
	BankPageFontSize      int  `json:"bankPageFontSize"`
}

// WaveformEngine is one entry in the waveform engine table (frame
// 03 24; the MC8 Pro has 8 engines). The dump carries the engine
// number on the wire; the write direction (04 05) omits it and
// relies on array order.
type WaveformEngine struct {
	EngineNum int `json:"num"`
	Min       int `json:"min"`  // 0–127
	Max       int `json:"max"`  // 0–127
	Type      int `json:"type"` // waveform type
}

// ResistorLadderSwitch is one aux switch calibration entry (frame
// 03 28; the MC8 Pro has 16).
type ResistorLadderSwitch struct {
	SwitchNumber int `json:"switchNumber"`
	TriggerValue int `json:"triggerValue"`
	F1           int `json:"f1"`
	F2           int `json:"f2"`
}

// MidiClockSlot is one BPM preset in the MIDI clock table (03 29).
type MidiClockSlot struct {
	BPM int `json:"bpm"` // 0–500 (14-bit on wire: LSB | MSB<<7)
}

// ScrollCounter is one scroll counter slot (frame 03 26; the MC8 Pro
// has 16). A counter scrolls between Min and Max starting at Start.
type ScrollCounter struct {
	Min   int `json:"min"`   // 0–127
	Max   int `json:"max"`   // 0–127
	Start int `json:"start"` // initial counter value
}

// BankArrangement holds the bank ordering configuration (frame
// 03 22). BankOrder is the raw 128-slot order table from the wire:
// each entry is a bank index, with 127 marking an unused slot.
// NumBanksUsed is the length of the active list per the editor's
// read semantics (a count field of 0 means "scan all slots, skipping
// 127"). Note the wire cannot distinguish literal bank 127 in the
// order table from the unused sentinel — an editor limitation the
// SDK inherits.
type BankArrangement struct {
	IsActive     bool     `json:"isActive"`
	NumBanksUsed int      `json:"numBanksUsed"` // number of active banks
	BankOrder    [128]int `json:"bankOrder"`    // bank indices; 127 = unused
}

// SequencerEngine is one entry in the sequencer engine table (frame
// 03 25; the MC8 Pro has 8). Each engine plays Arr[0..Len] in order.
// The engine number is not on the wire in either direction — it is
// the array index — but is kept here to match the JSON schema's
// engineNum field.
type SequencerEngine struct {
	EngineNum int     `json:"engineNum"`
	Len       int     `json:"len"` // 0–15; last used index into Arr
	Arr       [16]int `json:"arr"`
}

// OmniportInput is one expression/omniport entry (frame 03 23; the
// MC8 Pro has 4). Field names and JSON keys match the editor's
// omniport_input schema: each of the tip / ring / tip-ring contacts
// has a fixed-switch mode plus two data values.
type OmniportInput struct {
	PortNum        int `json:"portNum"`
	Type           int `json:"type"`
	FixedSwTip     int `json:"fixedSwTip"`
	TipData1       int `json:"td1"`
	TipData2       int `json:"td2"`
	FixedSwRing    int `json:"fixedSwRing"`
	RingData1      int `json:"rd1"`
	RingData2      int `json:"rd2"`
	FixedSwTipRing int `json:"fixedSwTipRing"`
	TipRingData1   int `json:"trd1"`
	TipRingData2   int `json:"trd2"`
}

// MidiEvent is one MIDI event processor rule (frame 03 27; the MC8
// Pro has 16 slots). Each rule matches incoming MIDI by type /
// number / channel / value and rewrites it to the To values.
type MidiEvent struct {
	TypeFrom           int  `json:"typeFrom"`
	TypeTo             int  `json:"typeTo"`
	NumberFrom         int  `json:"numberFrom"`
	NumberTo           int  `json:"numberTo"`
	ChannelFrom        int  `json:"channelFrom"`
	ChannelTo          int  `json:"channelTo"`
	ValueFrom          int  `json:"valueFrom"`
	ValueTo            int  `json:"valueTo"`
	ToSetOutgoingValue bool `json:"toSetOutgoingValue"`
	ToMapInputOutput   int  `json:"toMapInputOutput"` // 2-bit, 0–3
	ToMapValue         bool `json:"toMapValue"`
}

// MidiChannel is one MIDI channel's configuration (03 20).
type MidiChannel struct {
	Name       string  `json:"name"` // 16-char ASCII
	Remap      int     `json:"remap"`
	PortMSB    int     `json:"portMsb"`
	PortLSB    int     `json:"portLsb"`
	Attributes [16]int `json:"attributes"` // 16 per-channel flags
}
