package buildoutput

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

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
	Msg      string
	Finished bool
}

type BuildOutput struct {
	Cmd      *exec.Cmd
	outputCh chan string
}

func New(cmd *exec.Cmd) *BuildOutput {
	return &BuildOutput{
		Cmd:      cmd,
		outputCh: make(chan string, 99999),
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

	stdScanner := bufio.NewScanner(stdout)
	errScanner := bufio.NewScanner(stdErr)

	go func() {
		for stdScanner.Scan() {
			b.outputCh <- stdScanner.Text()
		}
	}()

	go func() {
		for errScanner.Scan() {
			b.outputCh <- errScanner.Text()
		}
	}()

	if err := b.Cmd.Start(); err != nil {
		return func() tea.Msg {
			return fmt.Errorf("failed to start command: %w", err)
		}
	}

	return func() tea.Msg {
		return BuildStarted
	}
}

func (b *BuildOutput) Update(msg tea.Msg) tea.Cmd {
	return b.readNextLine
}

func (b *BuildOutput) readNextLine() tea.Msg {
	msg := <-b.outputCh

	var finished bool
	if strings.Contains(msg, "✅ Build complete!") {
		msg += " Press <r> to start your substreams request (or <tab> to navigate)"
		finished = true

	} else if strings.Contains(msg, "❌ Error building spkg:") {
		msg += " Press <b> when ready to build again (or <tab> to navigate)"
		finished = true
	}

	return BuildOutputMsg{
		Msg:      msg,
		Finished: finished,
	}
}
