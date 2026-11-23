// Package tui is SENTRIX's terminal interface. It uses only ANSI escape
// codes and the standard library; with colour disabled (NO_COLOR, TERM=dumb,
// piped output or theme "mono") it degrades to plain ASCII.
package tui

import (
	"fmt"
	"os"
)

type Theme struct{ Color bool }

// DetectTheme enables colour only for interactive terminals.
func DetectTheme(setting string, out *os.File) Theme {
	if setting == "mono" || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return Theme{}
	}
	fi, err := out.Stat()
	return Theme{Color: err == nil && fi.Mode()&os.ModeCharDevice != 0}
}

func (t Theme) paint(code, s string) string {
	if !t.Color {
		return s
	}
	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", code, s)
}

func (t Theme) Accent(s string) string { return t.paint("1;31", s) }
func (t Theme) Bold(s string) string   { return t.paint("1", s) }
func (t Theme) Dim(s string) string    { return t.paint("90", s) }
func (t Theme) OK(s string) string     { return t.paint("32", s) }
func (t Theme) Warn(s string) string   { return t.paint("33", s) }
func (t Theme) Err(s string) string    { return t.paint("31", s) }

// Mark returns a status marker; ASCII-only when colour is off.
func (t Theme) Mark(kind string) string {
	if !t.Color {
		return map[string]string{"ok": "[OK]", "fail": "[!!]", "run": "[..]", "skip": "[--]"}[kind]
	}
	switch kind {
	case "ok":
		return t.OK("✔")
	case "fail":
		return t.Err("✖")
	case "skip":
		return t.Warn("●")
	}
	return t.Dim("…")
}

// Header prints a section title.
func (t Theme) Header(s string) string {
	return "\n" + t.Accent("▌ ") + t.Bold(s) + "\n"
}
