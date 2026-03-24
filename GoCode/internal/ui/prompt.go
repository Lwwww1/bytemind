package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Prompter struct {
	reader *bufio.Reader
	out    io.Writer
}

func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{
		reader: bufio.NewReader(in),
		out:    out,
	}
}

func (p *Prompter) ReadLine(prompt string) (string, error) {
	if _, err := fmt.Fprint(p.out, prompt); err != nil {
		return "", err
	}
	line, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	trimmed := strings.TrimRight(line, "\r\n")
	if errors.Is(err, io.EOF) && trimmed == "" {
		return "", io.EOF
	}
	return trimmed, nil
}

func (p *Prompter) Confirm(prompt string) (bool, error) {
	line, err := p.ReadLine(prompt + " [y/N]: ")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
