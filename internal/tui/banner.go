package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"sentrix/internal/buildinfo"
)

var glyphs = map[rune][6]string{
	'S': {"███████╗", "██╔════╝", "███████╗", "╚════██║", "███████║", "╚══════╝"},
	'E': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},
	'N': {"███╗   ██╗", "████╗  ██║", "██╔██╗ ██║", "██║╚██╗██║", "██║ ╚████║", "╚═╝  ╚═══╝"},
	'T': {"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   "},
	'R': {"██████╗ ", "██╔══██╗", "██████╔╝", "██╔══██╗", "██║  ██║", "╚═╝  ╚═╝"},
	'I': {"██╗", "██║", "██║", "██║", "██║", "╚═╝"},
	'X': {"██╗  ██╗", "╚██╗██╔╝", " ╚███╔╝ ", " ██╔██╗ ", "██╔╝ ██╗", "╚═╝  ╚═╝"},
}

func logoRows() []string {
	rows := make([]string, 6)
	for _, c := range "SENTRIX" {
		g := glyphs[c]
		for i := range rows {
			rows[i] += g[i]
		}
	}
	return rows
}

// Banner prints the startup banner. When animate is true the logo is revealed
// row by row; the animation is purely cosmetic and never reports progress.
func Banner(w io.Writer, t Theme, animate bool) {
	if t.Color {
		fmt.Fprintln(w)
		for _, r := range logoRows() {
			fmt.Fprintln(w, t.Accent(r))
			if animate {
				time.Sleep(60 * time.Millisecond)
			}
		}
	} else {
		fmt.Fprintf(w, "\n=== %s ===\n", buildinfo.Name)
	}
	fmt.Fprintf(w, "\n  %s\n\n", t.Bold(buildinfo.Subtitle))
	fmt.Fprintf(w, "  ENGINEERED BY %s\n  %s\n\n  GitHub: %s\n", buildinfo.Developer, buildinfo.Org, buildinfo.GitHub)
	fmt.Fprintf(w, "  %s\n\n", t.Dim("v"+buildinfo.Version+" | passive reconnaissance for authorized testing only"))
}

// About prints the About screen.
func About(w io.Writer, t Theme) {
	fmt.Fprint(w, t.Header("About "+buildinfo.Name))
	fmt.Fprintf(w, "%s\n%s\n\n", buildinfo.Name+" - "+buildinfo.Subtitle, strings.Repeat("-", 44))
	fmt.Fprintf(w, "Version:    %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
	fmt.Fprintf(w, "Developer:  %s (alias %s)\n", buildinfo.Developer, buildinfo.Alias)
	fmt.Fprintf(w, "Lab:        %s\n", buildinfo.Org)
	fmt.Fprintf(w, "GitHub:     %s\n\n", buildinfo.GitHub)
	fmt.Fprintln(w, "An independent, modular, open-source passive reconnaissance tool for authorized")
	fmt.Fprintln(w, "bug bounty researchers and security professionals. It queries public datasets;")
	fmt.Fprintln(w, "it never contacts the target and never tests for vulnerabilities.")
}
