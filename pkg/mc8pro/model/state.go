package model

// State is the runtime view of one Morningstar MC8 Pro session. It is
// populated by the SDK's [Client.Open] during the initial dump
// collection, and updated by [Client.SelectBank] when the user
// navigates between banks.
//
// State is distinct from [Dump]. Dump describes a JSON backup file's
// structure; State describes the live device at a moment in time. They
// share most field types (Bank, Preset, Message, Device) but State
// carries fewer fields because the device doesn't dump everything in
// one go — only the currently-focused bank and some global context.
//
// Lifecycle:
//
//	At Open() time, the device sends a ~39-frame cascade containing
//	firmware info, controller settings, bank names, and the currently-
//	selected bank's full data. State.Bank holds that bank; State.Device
//	and State.BankNames hold the device-wide context.
//
//	After Open, the caller can use Client.SelectBank to switch banks.
//	Each switch triggers a smaller (~3-frame) dump from the device; the
//	SDK ingests it into State.Bank, updates State.CurrentBank, and
//	leaves State.BankNames and State.Device unchanged.
//
// The editor's UI is driven by exactly this data model: BankNames
// populates the bank dropdown, Bank populates the preset grid, Device
// populates the firmware header.
type State struct {
	// Device metadata populated from the 11 03 firmware frame.
	// Stable for the lifetime of the session.
	Device Device

	// BankNames holds the names of all 128 banks, indexed by bank
	// number. Populated from the 11 05 bank-names frames. Only bank
	// slots with a non-empty name are meaningfully populated; the
	// rest are space-padded empty strings that decodeASCII trims to
	// "". This is used by the bank-picker UI.
	BankNames [128]string

	// Bank is the currently-focused bank's full data: preset array,
	// expression presets, bank-level messages, bank colors, and the
	// bank name (which should match BankNames[CurrentBank]).
	Bank Bank

	// CurrentBank is the index of the bank currently displayed on
	// the device's LCD and stored in [Bank]. Valid range 0..127.
	// The MC8 Pro has been observed to report 127 (0x7F) as a real
	// bank index on first connect if the user was physically on
	// the last bank; it is NOT a sentinel.
	CurrentBank int

	// UUID is the controller's unique identifier from the 11 00
	// frame: 32 nibble-encoded wire bytes → 16 bytes, rendered here
	// as a 32-char lowercase hex string of all 16 bytes. NOTE: this
	// is NOT the string the editor displays or uses as its cloud
	// profile key — for Pro models the editor renders only the first
	// 7 bytes, hyphen-separated with byte 6 repeated
	// ("b0-b1-b2-b3-b4-b5-b6-b6", editor.js:11511). Derive that form
	// from this one if editor parity is needed.
	UUID string

	// Controller holds global device settings from the 03 21 frame.
	Controller ControllerConfig

	// MidiClockSlots from the 03 29 frame (NOT 03 28 — corrected
	// with the controller-settings frame remap; 03 28 carries
	// resistor-ladder data and is currently stashed in Raw).
	MidiClockSlots []MidiClockSlot

	// MidiChannels from the 03 20 frame (16 channels).
	MidiChannels [16]MidiChannel

	// Controller-settings sections, one per source frame (03 22 bank
	// arrangement, 03 23 omniports, 03 24 waveform, 03 25 sequencer,
	// 03 26 scroll counters, 03 27 event processor, 03 28 resistor
	// ladder — per the editor's dispatch, editor.js:91104-91152).
	WaveformEngines  []WaveformEngine
	ResistorLadder   []ResistorLadderSwitch
	BankArrangement  BankArrangement
	SequencerEngines []SequencerEngine
	ScrollCounters   []ScrollCounter
	Omniports        []OmniportInput
	MidiEvents       [16]MidiEvent

	// Raw holds opaque bytes for inbound frame types the SDK does
	// not fully decode. Keys are (cmd1 << 8) | cmd2; values are the
	// full frame payloads. As decoding expands, this map shrinks.
	Raw map[uint16][]byte
}

// NewState returns an initialized State with no frames ingested.
// Raw is allocated to a non-nil empty map so callers can insert
// without a nil check.
func NewState() State {
	return State{
		Raw: make(map[uint16][]byte),
	}
}

// Clone returns a deep copy of s. Use this when exposing State to
// external callers through an accessor, to prevent accidental
// mutation of the SDK's internal state.
//
// The [128]string and Bank fields are copied by value; Raw is
// copied key-by-key with fresh byte slices so the returned State
// shares nothing with the original.
func (s State) Clone() State {
	out := State{
		Device:          s.Device,
		UUID:            s.UUID,
		BankNames:       s.BankNames,
		Bank:            s.Bank, // Bank is all arrays / values, no slices
		CurrentBank:     s.CurrentBank,
		Controller:      s.Controller,
		BankArrangement: s.BankArrangement,
		MidiEvents:      s.MidiEvents,
		MidiChannels:    s.MidiChannels,
		Raw:             make(map[uint16][]byte, len(s.Raw)),
	}
	// Deep-copy slices.
	if s.WaveformEngines != nil {
		out.WaveformEngines = make([]WaveformEngine, len(s.WaveformEngines))
		copy(out.WaveformEngines, s.WaveformEngines)
	}
	if s.ResistorLadder != nil {
		out.ResistorLadder = make([]ResistorLadderSwitch, len(s.ResistorLadder))
		copy(out.ResistorLadder, s.ResistorLadder)
	}
	if s.MidiClockSlots != nil {
		out.MidiClockSlots = make([]MidiClockSlot, len(s.MidiClockSlots))
		copy(out.MidiClockSlots, s.MidiClockSlots)
	}
	if s.SequencerEngines != nil {
		out.SequencerEngines = make([]SequencerEngine, len(s.SequencerEngines))
		copy(out.SequencerEngines, s.SequencerEngines)
	}
	if s.ScrollCounters != nil {
		out.ScrollCounters = make([]ScrollCounter, len(s.ScrollCounters))
		copy(out.ScrollCounters, s.ScrollCounters)
	}
	if s.Omniports != nil {
		out.Omniports = make([]OmniportInput, len(s.Omniports))
		copy(out.Omniports, s.Omniports)
	}
	for k, v := range s.Raw {
		cp := make([]byte, len(v))
		copy(cp, v)
		out.Raw[k] = cp
	}
	return out
}
