// Package mc8pro is a Go SDK for the Morningstar MC8 Pro MIDI controller.
//
// It exposes a typed data model for the device's banks, presets, and
// messages (matching the JSON schema of the official Morningstar web
// editor's backup files), plus a [Client] for live device I/O over
// USB MIDI.
//
// The package targets the MC8 Pro specifically. Other devices in the
// Morningstar gen2 family (MC3, MC6, MC6 Pro, gen1 MC8, etc.) share
// much of the underlying protocol but are not currently supported;
// they will live in sibling packages such as mc6pro when added.
//
// Internal layering:
//
//   - [model] holds the pure data types with no other dependencies.
//   - [sysex] is the wire codec; depends only on [model].
//   - This package depends on both and adds the live MIDI Client.
//
// User code should import only this package. The data types defined
// in [model] are re-exported here as type aliases, so writing
// mc8pro.Bank is exactly equivalent to writing model.Bank.
package mc8pro

import "github.com/benemon/morningstar-sdk/pkg/mc8pro/model"

// Re-exports of every public type from the model subpackage. These
// are Go type aliases (note the `=`), which means the alias and the
// original are the same type for all purposes — assignment, type
// assertion, reflection. Callers don't need to know model exists.
type (
	Dump                    = model.Dump
	DumpData                = model.DumpData
	Bank                    = model.Bank
	Preset                  = model.Preset
	Message                 = model.Message
	Device                  = model.Device
	Version                 = model.Version
	State                   = model.State
	ControllerSettings      = model.ControllerSettings
	ControllerSettingsData  = model.ControllerSettingsData
	OpaqueSection           = model.OpaqueSection
	BankArrangementSection  = model.BankArrangementSection
)
