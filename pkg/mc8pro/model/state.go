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
	// number. Populated from the 03 20 bank-names frame. Only bank
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

	// Raw holds opaque bytes for every inbound frame type the SDK
	// does not yet decode. Keys are (cmd1 << 8) | cmd2; values are
	// the full frame payloads (not the 16-byte header). This lets
	// the SDK round-trip unknown data losslessly during writes
	// (Phase 4) without needing to understand every sub-section.
	//
	// As decoding support expands, entries move from Raw into
	// typed fields and this map shrinks.
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
		Device:      s.Device,
		BankNames:   s.BankNames,
		Bank:        s.Bank, // Bank is all arrays / values, no slices
		CurrentBank: s.CurrentBank,
		Raw:         make(map[uint16][]byte, len(s.Raw)),
	}
	for k, v := range s.Raw {
		cp := make([]byte, len(v))
		copy(cp, v)
		out.Raw[k] = cp
	}
	return out
}
