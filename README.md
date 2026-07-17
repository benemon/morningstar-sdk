# morningstar-sdk

A Go SDK for programmatic control of the **Morningstar MC8 Pro** MIDI
controller over USB SysEx — read and write banks, presets, messages,
and controller settings without the web editor.

> **This is an independent, unofficial project.** It is not affiliated
> with, endorsed by, or supported by Morningstar Engineering. All
> product names and trademarks are the property of their respective
> owners. Use at your own risk — see [Safety](#safety) below.

## What it does

The SDK speaks all three of the MC8 Pro's control planes:

- **Editor session protocol** (reverse-engineered): the full data
  plane used by the official web editor. Open a session, read every
  bank/preset/message, write presets, bank config, and all controller
  settings sections (omniports, waveform/sequencer engines, scroll
  counters, MIDI events, MIDI channels, clock slots, resistor ladder,
  bank arrangement), select banks, engage presets, and subscribe to
  live device events.
- **Official external SysEx API** (`0x70` command family, documented
  by Morningstar): granular, firmware-validated writes — preset
  names, PC/CC messages, preset options, bank name, LCD display,
  toggle states — with per-write control over save-to-flash versus
  temporary override.
- **Plain MIDI CC/PC control**: the device's documented incoming
  CC/PC map needs no SysEx at all; the SDK's catalogs
  (`model/enums.go`) cover the message types, actions, and toggle
  positions involved.

Message type, action, and toggle-position catalogs with display names
are included, matching the editor's UI labels.

## Status

Hardware-verified against an MC8 Pro on firmware **3.13.6**. Read and
write paths are validated with zero-diff read→write→read cycles on
real hardware, including a full-bank restore. Other Morningstar
controllers (MC3, MC6, MC8 non-Pro, MC6 Pro) share much of the
protocol but are not currently supported.

The public API is pre-1.0 and may still change between minor versions.

## Install

```sh
go get github.com/benemon/morningstar-sdk
```

The MIDI layer uses [gomidi](https://gitlab.com/gomidi/midi) with the
rtmidi driver, which requires cgo. On Linux, install the ALSA headers
first (`libasound2-dev` on Debian/Ubuntu). macOS needs no extra
dependencies.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
)

func main() {
	ctx := context.Background()

	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close() // always: leaving edit mode requires this

	dev := client.Device()
	fmt.Printf("MC8 Pro, firmware %d.%d.%d\n",
		dev.Firmware.Major, dev.Firmware.Minor, dev.Firmware.Patch)

	// Read every preset in the current bank.
	if err := client.ReadBank(ctx); err != nil {
		log.Fatal(err)
	}
}
```

`Open` puts the device into edit mode (the same mode the web editor
uses). **Always `defer client.Close()`** — a session left open keeps
the device stuck in edit mode until a power cycle or another tool
closes it.

## Command-line tools

- **`cmd/mcctl`** — shell-level device control: `list`, `ping`,
  `show bank`/`show preset` (deterministic, diff-friendly output),
  `set cc` / `set name` (granular writes via the official external
  API, saved to flash by default, `--temporary` for live-only
  overrides), and `watch` for live SysEx tailing.
- **`cmd/mccapture`** — captures device dumps into test fixtures.

```sh
go run ./cmd/mcctl show bank 0
go run ./cmd/mcctl set cc --bank 0 --preset A --slot 0 --channel 2 --cc 35 --value 64
```

## Testing

Unit tests run anywhere, no hardware needed — the fixtures in
`pkg/mc8pro/testdata/` (JSON backups and raw SysEx captures) are
committed for exactly this reason:

```sh
go test ./...
```

Integration tests exercise a live device and are gated behind a build
tag:

```sh
go test -tags=integration ./...
```

Before running integration tests: connect an MC8 Pro via USB,
disconnect the web editor, and close any MIDI monitoring tools
(macOS CoreMIDI port sharing causes frame loss during bulk dumps).
Integration tests follow a read → mutate → write → verify → restore
pattern and leave the device in its original state, but they do write
to the device — don't run them against a controller whose
configuration you haven't backed up.

## Safety

- **Writes go to device flash** (unless using the external API's
  temporary mode). The SDK is careful — granular writes touch only
  what changed, and write paths are hardware-verified — but back up
  your device with the Morningstar editor before scripting writes.
- **One process per port.** Only one application can hold the
  device's Port 1. Disconnect the web editor before using the SDK.
- **Sessions must be closed.** See the note under Quick start.

## Protocol provenance

The editor session protocol was reverse-engineered for
interoperability by observing the device's own USB MIDI traffic and
studying the publicly served web editor, then verified byte-by-byte
against live hardware. The `0x70` external command family follows
Morningstar's published *SysEx API for External Applications*
document. **This repository contains no Morningstar code or assets**
— only an independent implementation of the wire protocol, plus test
fixtures captured from the author's own device.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
