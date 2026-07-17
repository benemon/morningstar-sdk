// mcctl is a thin shell wrapper over the mc8pro SDK for one-shot
// device operations. Each subcommand opens a session, performs one
// operation, and closes cleanly. Run with the pedal connected and the
// web editor disconnected (only one process can hold MIDI Port 1).
//
//	mcctl list                                 List connected MC8 Pros
//	mcctl ping                                 Ping the device
//	mcctl show bank 0                          Print one bank as readable text
//	mcctl show preset 0 A                      Print one preset
//	mcctl set cc --bank 0 --preset A --slot 0 \
//	      --channel 2 --cc 35 --value 64       Write one CC message
//	mcctl set name --bank 0 --preset A "Clean" Write a preset short name
//	mcctl watch                                Live-tail SysEx traffic
//
// Writes use the official external SysEx API (0x70 family) and are
// saved to flash by default; --temporary applies a live override that
// reverts when the device changes bank. `show` output is
// deterministic across runs so it can be diffed.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro"
	"github.com/benemon/morningstar-sdk/pkg/mc8pro/model"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	switch os.Args[1] {
	case "list":
		cmdList()
	case "ping":
		cmdPing(ctx)
	case "show":
		cmdShow(ctx, os.Args[2:])
	case "set":
		cmdSet(ctx, os.Args[2:])
	case "watch":
		cmdWatch(ctx)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  mcctl list                                  list connected MC8 Pros
  mcctl ping                                  ping the device
  mcctl show bank <0-127>                     print one bank
  mcctl show preset <0-127> <A-X|0-23>        print one preset
  mcctl set cc --bank N --preset P --slot 0-15 --channel N --cc N --value N [--temporary]
  mcctl set name --bank N --preset P [--temporary] <name>
  mcctl watch                                 live-tail SysEx traffic
`)
	os.Exit(2)
}

func cmdList() {
	// Each MC8 Pro exposes four virtual MIDI ports; Port 1 is the
	// control port, so one Port 1 line = one physical device.
	found := 0
	for _, d := range mc8pro.ListDevices() {
		if strings.Contains(d.Name, "Port 1") {
			fmt.Println(d.Name)
			found++
		}
	}
	if found == 0 {
		fmt.Println("no MC8 Pro devices found")
	}
}

func cmdPing(ctx context.Context) {
	client := open(ctx)
	defer client.Close()

	active, err := client.Ping(ctx)
	if err != nil {
		die("ping: %v", err)
	}
	dev := client.Device()
	fmt.Printf("firmware %s  serial % X  session_active=%v\n",
		dev.Firmware, dev.Serial, active)
}

func cmdShow(ctx context.Context, args []string) {
	if len(args) < 2 {
		usage()
	}
	switch args[0] {
	case "bank":
		bank := parseBank(args[1])
		client := open(ctx)
		defer client.Close()
		state := selectBank(ctx, client, bank)
		printBank(state.Bank)
	case "preset":
		if len(args) < 3 {
			usage()
		}
		bank := parseBank(args[1])
		preset := parsePreset(args[2])
		client := open(ctx)
		defer client.Close()
		state := selectBank(ctx, client, bank)
		fmt.Printf("bank %d %q\n", state.Bank.BankNumber, state.Bank.BankName)
		printPreset(state.Bank.PresetArray[preset], false)
	default:
		usage()
	}
}

func cmdSet(ctx context.Context, args []string) {
	if len(args) < 1 {
		usage()
	}
	switch args[0] {
	case "cc":
		cmdSetCC(ctx, args[1:])
	case "name":
		cmdSetName(ctx, args[1:])
	default:
		usage()
	}
}

func cmdSetCC(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("set cc", flag.ExitOnError)
	bank := fs.Int("bank", -1, "bank index 0-127")
	preset := fs.String("preset", "", "preset letter A-X or index 0-23")
	slot := fs.Int("slot", -1, "message slot 0-15")
	channel := fs.Int("channel", -1, "MIDI channel 1-16")
	cc := fs.Int("cc", -1, "CC number 0-127")
	value := fs.Int("value", -1, "CC value 0-127")
	temporary := fs.Bool("temporary", false, "live override only; reverts on bank change")
	fs.Parse(args)

	bankNum := parseBankInt(*bank)
	presetNum := parsePreset(*preset)
	if *slot < 0 || *slot > 15 {
		die("set cc: --slot %d out of range 0..15 (the external API covers slots 0-15)", *slot)
	}
	if *channel < 1 || *channel > 16 {
		die("set cc: --channel %d out of range 1..16", *channel)
	}
	if *cc < 0 || *cc > 127 {
		die("set cc: --cc %d out of range 0..127", *cc)
	}
	if *value < 0 || *value > 127 {
		die("set cc: --value %d out of range 0..127", *value)
	}

	client := open(ctx)
	defer client.Close()
	selectBank(ctx, client, bankNum)

	// The message fires on Press in both toggle positions — the
	// editor's defaults for a new message.
	msg := mc8pro.Message{
		Type:    model.MsgTypeCC,
		Action:  model.ActionPress,
		Toggle:  model.TogglePosBoth,
		Channel: *channel,
	}
	msg.Data[0] = *cc
	msg.Data[1] = *value
	if err := client.SetPresetMessage(ctx, presetNum, *slot, msg, !*temporary); err != nil {
		die("set cc: %v", err)
	}
	settle(ctx, *temporary)
	fmt.Printf("bank %d preset %s slot %d: CC ch=%d cc=%d val=%d written%s\n",
		bankNum, presetLabel(presetNum), *slot, *channel, *cc, *value, temporarySuffix(*temporary))
}

func cmdSetName(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("set name", flag.ExitOnError)
	bank := fs.Int("bank", -1, "bank index 0-127")
	preset := fs.String("preset", "", "preset letter A-X or index 0-23")
	temporary := fs.Bool("temporary", false, "live override only; reverts on bank change")
	fs.Parse(args)
	if fs.NArg() != 1 {
		die("set name: exactly one name argument required")
	}
	name := fs.Arg(0)

	bankNum := parseBankInt(*bank)
	presetNum := parsePreset(*preset)

	client := open(ctx)
	defer client.Close()
	selectBank(ctx, client, bankNum)

	if err := client.SetPresetShortName(ctx, presetNum, name, !*temporary); err != nil {
		die("set name: %v", err)
	}
	settle(ctx, *temporary)
	fmt.Printf("bank %d preset %s: name %q written%s\n",
		bankNum, presetLabel(presetNum), name, temporarySuffix(*temporary))
}

func cmdWatch(ctx context.Context) {
	client := open(ctx)
	defer client.Close()

	ch, cancel := client.Subscribe(nil)
	defer cancel()

	fmt.Fprintln(os.Stderr, "watching (ctrl-c to stop)...")
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			fmt.Printf("%s cmd=%02X %02X args=% X payload=%d bytes\n",
				time.Now().Format("15:04:05.000"),
				frame.Cmd1, frame.Cmd2, frame.Args, len(frame.Payload))
		}
	}
}

// open starts a session with default options and dies on failure.
func open(ctx context.Context) *mc8pro.Client {
	client, err := mc8pro.Open(ctx, mc8pro.OpenOptions{})
	if err != nil {
		die("open: %v", err)
	}
	return client
}

// selectBank navigates the client to a bank (which also reads the
// full bank data) and returns the resulting state.
func selectBank(ctx context.Context, client *mc8pro.Client, bank int) mc8pro.State {
	if err := client.SelectBank(ctx, bank); err != nil {
		die("select bank %d: %v", bank, err)
	}
	return client.State()
}

// settle waits out the device's write-apply (and, for saved writes,
// flash-commit) latency before the caller closes the session.
func settle(ctx context.Context, temporary bool) {
	d := 1500 * time.Millisecond
	if temporary {
		d = 400 * time.Millisecond
	}
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func temporarySuffix(temporary bool) string {
	if temporary {
		return " (temporary; reverts on bank change)"
	}
	return ""
}

func printBank(b mc8pro.Bank) {
	fmt.Printf("bank %d %q\n", b.BankNumber, b.BankName)
	if b.BankDescription != "" {
		fmt.Printf("description %q\n", b.BankDescription)
	}
	for _, m := range b.BankMsgArray {
		if m.Type != model.MsgTypeEmpty {
			fmt.Printf("bank msg %d: %s\n", m.M, formatMessage(m, false))
		}
	}
	for _, p := range b.PresetArray {
		printPreset(p, false)
	}
	for _, p := range b.ExpPresetArray {
		printPreset(p, true)
	}
}

func printPreset(p mc8pro.Preset, isExp bool) {
	label := presetLabel(p.PresetNum)
	if isExp {
		label = fmt.Sprintf("EXP%d", p.PresetNum+1)
	}
	fmt.Printf("preset %s %q", label, p.ShortName)
	if p.ToggleName != "" {
		fmt.Printf(" toggle_name=%q", p.ToggleName)
	}
	if p.ToToggle {
		fmt.Printf(" toggle=on group=%d", p.ToggleGroup)
	}
	fmt.Println()
	for _, m := range p.MsgArray {
		if m.Type != model.MsgTypeEmpty {
			fmt.Printf("  msg %d: %s\n", m.M, formatMessage(m, isExp))
		}
	}
}

// formatMessage renders one message deterministically using the
// catalog names. Plain CC (the 90% path) gets fully named fields;
// every other type prints its display name plus the raw data with
// trailing zeros trimmed. Expression messages use their own type
// catalog — their Type numbering differs from preset/bank messages.
func formatMessage(m mc8pro.Message, isExp bool) string {
	action := model.ActionNames[m.Action]
	if action == "" {
		action = fmt.Sprintf("action-%d", m.Action)
	}
	toggle := model.TogglePosNames[m.Toggle]
	if toggle == "" {
		toggle = fmt.Sprintf("toggle-%d", m.Toggle)
	}
	if !isExp && m.Type == model.MsgTypeCC {
		return fmt.Sprintf("CC ch=%d cc=%d val=%d on=%q pos=%q",
			m.Channel, m.Data[0], m.Data[1], action, toggle)
	}
	typeNames := model.MsgTypeNames
	if isExp {
		typeNames = model.ExpMsgTypeNames
	}
	name := typeNames[m.Type]
	if name == "" {
		name = fmt.Sprintf("type-%d", m.Type)
	}
	data := m.Data[:]
	end := len(data)
	for end > 0 && data[end-1] == 0 {
		end--
	}
	return fmt.Sprintf("%s ch=%d on=%q pos=%q data=%v",
		name, m.Channel, action, toggle, data[:end])
}

// presetLabel renders a main preset index as its footswitch letter
// (A..X: 8 switches × 3 pages).
func presetLabel(n int) string {
	return string(rune('A' + n))
}

// parsePreset accepts a footswitch letter (A..X, case-insensitive)
// or a numeric index (0..23).
func parsePreset(s string) int {
	if s == "" {
		die("preset required (letter A-X or index 0-23)")
	}
	up := strings.ToUpper(s)
	if len(up) == 1 && up[0] >= 'A' && up[0] <= 'X' {
		return int(up[0] - 'A')
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 23 {
		die("invalid preset %q (want letter A-X or index 0-23)", s)
	}
	return n
}

func parseBank(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		die("invalid bank %q (want index 0-127)", s)
	}
	return parseBankInt(n)
}

func parseBankInt(n int) int {
	if n < 0 || n > 127 {
		die("bank %d out of range 0..127", n)
	}
	return n
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
