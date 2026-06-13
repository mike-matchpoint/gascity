package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/events"
)

// newBeadsOnCloseCmd is the consolidated bd on_close hook entry point.
// It replaces the four-process hook chain (gc event emit bead.closed +
// gc convoy autoclose + gc wisp autoclose + gc molecule autoclose) with
// one invocation that opens the bead store and the event sink once.
// Every gc startup costs hundreds of filesystem operations, which on
// NFS-backed hosted workspaces are network round trips each — bead
// closes are the hottest hook, so the 4x process fan-out was a dominant
// metadata-op generator.
func newBeadsOnCloseCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:    "on-close <bead-id>",
		Short:  "Run all bd on_close hook steps in one process",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			doBeadsOnClose(args[0], os.Stdin, stdout, stderr)
			return nil // always succeed — best-effort infrastructure
		},
	}
}

// doBeadsOnClose runs the on_close steps in hook order: emit
// bead.closed, then convoy/wisp/molecule autoclose. Each step keeps the
// exact semantics of its standalone command: the emit goes through the
// configured events provider (exec:/fake/file), the autoclosers write
// their events to the city file recorder, and every failure is
// swallowed best-effort.
func doBeadsOnClose(beadID string, stdin io.Reader, stdout, stderr io.Writer) {
	raw, _ := io.ReadAll(stdin)

	ep, _ := openCityEventEmitProvider(stderr, "gc beads on-close")
	if ep != nil {
		defer ep.Close() //nolint:errcheck // best-effort
		emitBeadClosed(ep, beadID, raw, stderr)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	storeRoot := convoyAutocloseStoreRoot(cwd)
	cityPath := autocloseCityPathForStoreRoot(storeRoot)
	store, err := openStoreAtForCity(storeRoot, cityPath)
	if err != nil {
		return
	}

	// Reuse the emit provider as the autoclose recorder when it is the
	// city file recorder (the default). Under exec:/fake providers the
	// autoclosers keep writing to the file recorder, matching their
	// standalone behavior.
	var rec events.Recorder
	if fr, ok := ep.(*events.FileRecorder); ok {
		rec = fr
	} else {
		rec = openCityRecorderAt(cityPath, stderr)
	}

	doBeadsOnCloseAutoclosers(store, rec, beadID, stdout, stderr)
}

// doBeadsOnCloseAutoclosers runs the three autoclosers in hook order
// against an already-open store and recorder.
func doBeadsOnCloseAutoclosers(store beads.Store, rec events.Recorder, beadID string, stdout, stderr io.Writer) {
	doConvoyAutocloseWith(store, rec, beadID, stdout, stderr)
	doWispAutocloseWith(store, beadID, stdout)
	doMoleculeAutocloseWith(store, rec, beadID, stdout)
}

// emitBeadClosed mirrors the hook's previous `gc event emit bead.closed
// --subject <id> --message <title> --payload {"bead":<json>}` call,
// including its behavior on malformed stdin: doEventEmit validates the
// payload and drops the event when the issue JSON is not valid.
func emitBeadClosed(ep events.Provider, beadID string, raw []byte, stderr io.Writer) {
	title := ""
	if json.Valid(raw) {
		var issue struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(raw, &issue); err == nil {
			title = issue.Title
		}
	}
	payload := fmt.Sprintf(`{"bead":%s}`, raw)
	doEventEmit(ep, "bead.closed", beadID, title, "", payload, stderr)
}
