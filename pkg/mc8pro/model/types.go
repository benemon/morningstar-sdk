// Package model holds the pure data types for the Morningstar MC8 Pro
// SDK: Bank, Preset, Message, Device, and the JSON dump structures.
//
// This package has no dependencies beyond the standard library and
// MUST NOT import any sibling package. It exists as the bottom of the
// dependency stack so both the wire codec (sysex) and the public
// client (mc8pro) can import it without creating cycles.
//
// User code should not import this package directly. The parent
// mc8pro package re-exports every type defined here via type aliases,
// so callers write mc8pro.Bank rather than model.Bank.
package model

// Device identifies one physical MC8 Pro discovered on the host.
// It's populated during [Client.Open] from the device's response to
// REQUEST_CONTROLLER_FIRMWARE_VERSION (frame command 11 03).
type Device struct {
	Model    int     // 8 = MC8 Pro
	Firmware Version // e.g. 3.13.6
	Serial   [4]byte // 4-byte device serial number, e.g. 77 4D 26 15
}

// Version is a major.minor.patch firmware triple.
type Version struct {
	Major uint8
	Minor uint8
	Patch uint8
}

// String formats a Version as "major.minor.patch".
func (v Version) String() string {
	return itoa(v.Major) + "." + itoa(v.Minor) + "." + itoa(v.Patch)
}

// itoa is a tiny helper so this file doesn't have to import strconv
// just for one call.
func itoa(b uint8) string {
	if b == 0 {
		return "0"
	}
	var buf [3]byte
	n := len(buf)
	for b > 0 {
		n--
		buf[n] = byte('0' + b%10)
		b /= 10
	}
	return string(buf[n:])
}

// Dump is the top-level structure of a Morningstar JSON backup file.
// Exactly one of [DumpData.Bank] or [DumpData.BankArray] is populated,
// as determined by [Dump.DumpType].
type Dump struct {
	SchemaVersion int      `json:"schemaVersion"`
	DumpType      string   `json:"dumpType"` // "singleBank" or "allBanks"
	DeviceModel   int      `json:"deviceModel"`
	DownloadDate  string   `json:"downloadDate"` // ISO 8601 string; kept as string for fidelity
	Hash          int64    `json:"hash"`
	Data          DumpData `json:"data"`
	Description   string   `json:"description"`
}

// DumpData is the variant body of a [Dump]. For a "singleBank" dump,
// [DumpData.Bank] is populated and the other fields are zero. For an
// "allBanks" dump, [DumpData.BankArray] and [DumpData.ControllerSettings]
// are populated and [DumpData.Bank] is nil.
type DumpData struct {
	Bank               *Bank               `json:"bank,omitempty"`
	BankArray          []Bank              `json:"bankArray,omitempty"`
	ControllerSettings *ControllerSettings `json:"controller_settings,omitempty"`
}

// Bank is one of the 128 configurable banks on an MC8 Pro. Each bank
// holds 24 preset slots (8 footswitches × 3 pages), 4 expression
// presets, and 32 bank-level messages that fire on bank entry/exit.
type Bank struct {
	BankNumber      int         `json:"bankNumber"` // 0..127
	BankName        string      `json:"bankName"`   // ≤16 ASCII chars
	BankClearToggle bool        `json:"bankClearToggle"`
	BankMsgArray    [32]Message `json:"bankMsgArray"`
	PresetArray     [24]Preset  `json:"presetArray"`
	ExpPresetArray  [4]Preset   `json:"expPresetArray"`
	BankDescription string      `json:"bankDescription"`
	ToDisplay       bool        `json:"toDisplay"`
	PageLimit       int         `json:"pageLimit"`
	BackgroundColor int         `json:"backgroundColor"`
	TextColor       int         `json:"textColor"`
	IsColorEnabled  bool        `json:"isColorEnabled"`
}

// Preset is one addressable footswitch state. An MC8 Pro bank has 24
// primary presets (PresetNum 0..23) and 4 expression presets
// (IsExp=true). Each preset has up to 32 messages which fire in
// response to actions (press, release, long-press, etc).
type Preset struct {
	PresetNum             int         `json:"presetNum"`
	BankNum               int         `json:"bankNum"`
	IsExp                 bool        `json:"isExp"`
	ShortName             string      `json:"shortName"` // ≤32 ASCII chars, shown on LCD
	ToggleName            string      `json:"toggleName"`
	LongName              string      `json:"longName"`
	ShiftName             string      `json:"shiftName"`
	ToToggle              bool        `json:"toToggle"`
	ToBlink               bool        `json:"toBlink"`
	ToMsgScroll           bool        `json:"toMsgScroll"`
	ToggleGroup           int         `json:"toggleGroup"`
	LedColor              int         `json:"ledColor"`
	LedToggleColor        int         `json:"ledToggleColor"`
	LedShiftColor         int         `json:"ledShiftColor"`
	NameColor             int         `json:"nameColor"`
	NameToggleColor       int         `json:"nameToggleColor"`
	NameShiftColor        int         `json:"nameShiftColor"`
	BackgroundColor       int         `json:"backgroundColor"`
	ToggleBackgroundColor int         `json:"toggleBackgroundColor"`
	ShiftBackgroundColor  int         `json:"shiftBackgroundColor"`
	MsgArray              [32]Message `json:"msgArray"`
}

// Message is one MIDI action fired by a preset. The meaning of Data
// depends on Type: for CC messages (Type=2), Data[0] is the CC number
// and Data[1] is the CC value; for other types the layout is
// type-specific and should be preserved opaquely.
//
// The 18-byte Data array is encoded on the wire across offsets [2..4]
// and [8..22] of the 23-byte SysEx message row — see CLAUDE.md for the
// full layout.
type Message struct {
	Data        [18]int `json:"data"`
	M           int     `json:"m"` // slot index 0..31 (matches array position)
	Channel     int     `json:"c"` // MIDI channel 1..16
	Type        int     `json:"t"` // 0=empty, 2=CC, 15=internal, 24=tap, ...
	Action      int     `json:"a"` // 1=press, 3=release, ...
	ToggleGroup int     `json:"tg"`
	Info        string  `json:"mi"` // optional message info text; may be client-side only
}

// ControllerSettings wraps the device-level configuration sub-sections
// present only in "allBanks" dumps. Each sub-section corresponds to
// one SysEx command family (e.g. omniports → 03 23). For now these are
// held as opaque wrappers; detailed decoding is deferred.
type ControllerSettings struct {
	Type string                 `json:"type"` // "controller_settings_all"
	Data ControllerSettingsData `json:"data"`
}

// ControllerSettingsData holds the ten sub-sections of a full-device
// controller-settings dump. Each field is held as a raw opaque section
// so reads and writes round-trip losslessly even before we decode the
// inner byte layout.
type ControllerSettingsData struct {
	Omniports          OpaqueSection          `json:"omniports"`
	ResistorLadderAux  OpaqueSection          `json:"resistor_ladder_aux"`
	ControllerSettings OpaqueSection          `json:"controller_settings"`
	WaveformEngines    OpaqueSection          `json:"waveform_engines"`
	SequencerEngines   OpaqueSection          `json:"sequencer_engines"`
	ScrollCounters     OpaqueSection          `json:"scroll_counters"`
	MidiChannels       OpaqueSection          `json:"midi_channels"`
	BankArrangement    BankArrangementSection `json:"bank_arrangement"`
	MidiEvents         OpaqueSection          `json:"midi_events"`
	MidiClockSlots     OpaqueSection          `json:"midi_clock_slots"`
}

// OpaqueSection is a controller-settings sub-section whose byte layout
// has not yet been decoded. It preserves the raw JSON for round-trip
// fidelity.
type OpaqueSection struct {
	Type string     `json:"type"`
	Data rawMessage `json:"data"`
}

// BankArrangementSection is special-cased because it carries two extra
// fields not present in other sections.
type BankArrangementSection struct {
	Type           string     `json:"type"`
	Data           rawMessage `json:"data"`
	IsActive       bool       `json:"isActive"`
	NumBanksActive int        `json:"numBanksActive"`
}

// rawMessage is an alias for json.RawMessage that we declare locally so
// we don't leak the encoding/json import into every file that uses the
// types package. It preserves the exact byte representation of a JSON
// sub-tree.
type rawMessage []byte

// MarshalJSON implements json.Marshaler for rawMessage by returning the
// stored bytes as-is (or "null" if empty).
func (r rawMessage) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}

// UnmarshalJSON implements json.Unmarshaler for rawMessage by capturing
// the raw input bytes verbatim.
func (r *rawMessage) UnmarshalJSON(data []byte) error {
	*r = append((*r)[:0], data...)
	return nil
}
