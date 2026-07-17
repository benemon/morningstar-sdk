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

// WaveformEngine is one entry in the waveform engine table
// (frame 03 24 per the corrected mapping; codec pending remap).
type WaveformEngine struct {
	Type int `json:"type"`
	Max  int `json:"max"` // 0–127
	Min  int `json:"min"` // 0–127
}

// ResistorLadderSwitch is one aux switch calibration entry
// (frame 03 28 per the corrected mapping; codec pending remap).
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

// BankArrangement holds the bank ordering configuration
// (frame 03 22 per the corrected mapping; codec pending remap).
type BankArrangement struct {
	IsActive     bool    `json:"isActive"`
	NumBanksUsed int     `json:"numBanksUsed"` // number of active banks
	BankOrder    [30]int `json:"bankOrder"`    // bank indices; 0x7F = unused
}

// SequencerEngine is one entry in the sequencer engine table
// (frame 03 25 per the corrected mapping; codec pending remap).
type SequencerEngine struct {
	EngineNum int    `json:"engineNum"`
	Len       int    `json:"len"`
	Arr       [9]int `json:"arr"` // 9 values on wire; JSON has 16 but [9..15] are zero
}

// OmniportInput is one expression/omniport entry
// (frame 03 23 per the corrected mapping; codec pending remap).
// Wire format: 4 bytes per port [portNum, type, val1, val2].
type OmniportInput struct {
	PortNum int `json:"portNum"`
	Type    int `json:"type"`
	Val1    int `json:"val1"`
	Val2    int `json:"val2"`
}

// MidiEventProcessor holds the MIDI event processor data
// (frame 03 27 per the corrected mapping; codec pending remap):
// a 10-byte configuration header and a 128-byte remap table where
// byte[i] = output value for input i. NOTE: this structure was
// derived from the 03 22 frame under the old wrong mapping and will
// likely change when re-derived from the editor's event-processor
// manager class.
type MidiEventProcessor struct {
	Header     [10]byte  `json:"header"`
	RemapTable [128]byte `json:"remapTable"`
}

// MidiChannel is one MIDI channel's configuration (03 20).
type MidiChannel struct {
	Name       string  `json:"name"` // 16-char ASCII
	Remap      int     `json:"remap"`
	PortMSB    int     `json:"portMsb"`
	PortLSB    int     `json:"portLsb"`
	Attributes [16]int `json:"attributes"` // 16 per-channel flags
}
