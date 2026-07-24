// charm — the Charm-stack candidate for bri.cli's interactive backend
// (spike s46). It links huh + bubbletea + lipgloss and exercises the widgets
// bri.cli's unified parameter model needs. `--measure` links everything
// without a TTY (for size/cgo/cross-compile measurement); with no flag it
// runs the interactive demo.
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
)

// promptText maps to a :string param's prompt (with inline validation — the
// same validator the CLI arg path runs, ADR 0078 §3).
func promptText(title string, validate func(string) error) (string, error) {
	var v string
	f := huh.NewInput().Title(title).Value(&v).Validate(validate)
	return v, f.Run()
}

// promptSelect maps to :enum/:one-of — arrow-key, filterable.
func promptSelect(title string, opts []string) (string, error) {
	var v string
	options := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		options[i] = huh.NewOption(o, o)
	}
	return v, huh.NewSelect[string]().Title(title).Options(options...).Value(&v).Run()
}

// promptConfirm maps to :bool.
func promptConfirm(title string) (bool, error) {
	var v bool
	return v, huh.NewConfirm().Title(title).Value(&v).Run()
}

// promptPassword maps to :secret — masked.
func promptPassword(title string) (string, error) {
	var v string
	return v, huh.NewInput().Title(title).EchoMode(huh.EchoModePassword).Value(&v).Run()
}

// promptMulti maps to :multi — multi-select set.
func promptMulti(title string, opts []string) ([]string, error) {
	var v []string
	options := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		options[i] = huh.NewOption(o, o)
	}
	return v, huh.NewMultiSelect[string]().Title(title).Options(options...).Value(&v).Run()
}

// withSpinner maps to ADR 0078 §4 progress/spinner.
func withSpinner(title string, work func()) error {
	return spinner.New().Title(title).Action(work).Run()
}

// style is lipgloss-as-data (ADR 0078 §4): {:bold true :fg :green}.
var okStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--measure" {
		// Reference every widget so the linker keeps the whole surface — the
		// realistic cost of adopting Charm. No TTY needed.
		_ = promptText
		_ = promptSelect
		_ = promptConfirm
		_ = promptPassword
		_ = promptMulti
		_ = withSpinner
		fmt.Println(okStyle.Render("charm: all widgets linked (measure mode)"))
		return
	}

	name, _ := promptText("Project name?", func(s string) error {
		if len(s) < 2 {
			return fmt.Errorf("must be at least 2 characters")
		}
		return nil
	})
	kind, _ := promptSelect("Template?", []string{"lib", "cli", "web"})
	feats, _ := promptMulti("Features?", []string{"otel", "db", "auth"})
	tok, _ := promptPassword("API token?")
	yes, _ := promptConfirm("Proceed?")
	_ = withSpinner("Building…", func() {})
	fmt.Println(okStyle.Render(fmt.Sprintf("name=%s kind=%s feats=%v toklen=%d yes=%v", name, kind, feats, len(tok), yes)))
}
