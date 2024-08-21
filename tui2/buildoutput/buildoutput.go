package buildoutput

import (
	"bufio"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type Msg int

const (
	BuildStarted Msg = iota
	BuildOngoing
	BuildDoneSuccess
	BuildDoneFailure
)

type BuildOutputMsg struct {
	Msg string
}

type BuildOutput struct {
	Cmd        *exec.Cmd
	StdScanner *bufio.Scanner
	ErrScanner *bufio.Scanner
}

func New(cmd *exec.Cmd) *BuildOutput {
	return &BuildOutput{
		Cmd: cmd,
	}
}

func (b *BuildOutput) Init() tea.Cmd {
	stdout, err := b.Cmd.StdoutPipe()
	if err != nil {
		return func() tea.Msg {
			return fmt.Errorf("failed to get stdout pipe: %w", err)
		}
	}

	stdErr, err := b.Cmd.StderrPipe()
	if err != nil {
		return func() tea.Msg {
			return fmt.Errorf("failed to get stderr pipe: %w", err)
		}
	}

	if err := b.Cmd.Start(); err != nil {
		return func() tea.Msg {
			return fmt.Errorf("failed to start command: %w", err)
		}
	}

	b.StdScanner = bufio.NewScanner(stdout)
	b.ErrScanner = bufio.NewScanner(stdErr)
	return func() tea.Msg {
		return BuildStarted
	}
}

func (b *BuildOutput) Update(msg tea.Msg) tea.Cmd {
	return b.readNextLine
}

func (b *BuildOutput) readNextLine() tea.Msg {
	if b.StdScanner.Scan() {
		return BuildOutputMsg{
			Msg: b.StdScanner.Text(),
		}
	}

	if b.StdScanner.Err() != nil {
		return BuildDoneFailure
	}

	return BuildDoneSuccess
}
