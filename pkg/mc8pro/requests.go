package mc8pro

import "github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"

// initRequest is one entry in the post-session-open request cascade
// the SDK fires to pull the device's full state. It mirrors the
// editor's requestControllerData() flow at editor.js:90802.
type initRequest struct {
	Name string
	Cmd2 byte
}

// initRequestSequence is the canonical list of REQUEST_* commands the
// editor fires on connect. We send the same sequence during
// Client.Open (and the mccapture tool does the same). Each command
// triggers one or more response frames which are collected by the
// dump-collection logic.
//
// Order matches editor.js:90802 requestControllerData() with two
// extensions: PRESET_NAMES and MIDI_CLOCK_SLOTS are appended because
// they produce additional data we want captured.
var initRequestSequence = []initRequest{
	{"UUID", sysex.CmdReqControllerUUID},
	{"FIRMWARE_VERSION", sysex.CmdReqControllerFirmwareVersion},
	{"BANK_ARRANGEMENT", sysex.CmdReqBankArrangement},
	{"EVENT_PROCESSOR", sysex.CmdReqEventProcessor},
	{"GENERAL_CONFIG", sysex.CmdReqControllerGeneralConfig},
	{"OMNIPORT_DATA", sysex.CmdReqOmniportData},
	{"WAVEFORM_ENGINE", sysex.CmdReqWaveformEngine},
	{"SCROLL_SLOTS", sysex.CmdReqScrollSlots},
	{"SEQUENCER_ENGINE", sysex.CmdReqSequencerEngine},
	{"MIDI_CHANNEL_NAMES", sysex.CmdReqMidiChannelNames},
	{"BANK_PRESET_NAMES", sysex.CmdReqBankPresetNames},
	{"CONTROLLER_SETTINGS_ALL", sysex.CmdReqControllerSettingsAll},
	{"PRESET_NAMES", sysex.CmdReqPresetNames},
	{"MIDI_CLOCK_SLOTS", sysex.CmdReqMidiClockSlots},
}

// InitRequestSequence exposes the request cascade as a slice of
// (name, cmd2) pairs so external tools like cmd/mccapture can
// iterate the same canonical list without duplication.
//
// Returns a copy so callers can't mutate the SDK's private table.
func InitRequestSequence() []struct {
	Name string
	Cmd2 byte
} {
	out := make([]struct {
		Name string
		Cmd2 byte
	}, len(initRequestSequence))
	for i, r := range initRequestSequence {
		out[i] = struct {
			Name string
			Cmd2 byte
		}{r.Name, r.Cmd2}
	}
	return out
}
