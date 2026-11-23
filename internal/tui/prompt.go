package tui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Prompter reads validated answers line by line. Line-based input works the
// same over SSH, in Windows consoles and in screen readers.
type Prompter struct {
	in  *bufio.Reader
	Out io.Writer
	T   Theme
}

func NewPrompter(in io.Reader, out io.Writer, t Theme) *Prompter {
	return &Prompter{in: bufio.NewReader(in), Out: out, T: t}
}

// Ask returns the trimmed answer, or def when the user just presses Enter.
// It returns io.EOF when input ends.
func (p *Prompter) Ask(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.Out, "%s %s ", p.T.Bold(label), p.T.Dim("["+def+"]"))
	} else {
		fmt.Fprintf(p.Out, "%s ", p.T.Bold(label))
	}
	line, err := p.in.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// AskValid repeats the question until validate accepts the answer.
func (p *Prompter) AskValid(label, def string, validate func(string) error) (string, error) {
	for {
		v, err := p.Ask(label, def)
		if err != nil {
			return "", err
		}
		if err := validate(v); err != nil {
			fmt.Fprintln(p.Out, "  "+p.T.Err("invalid: ")+err.Error())
			continue
		}
		return v, nil
	}
}

func (p *Prompter) Confirm(label string, def bool) (bool, error) {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	for {
		v, err := p.Ask(label+" ("+d+")", "")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(v) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}

// SplitList splits a comma-separated answer, dropping empty items.
func SplitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
