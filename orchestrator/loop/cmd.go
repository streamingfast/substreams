package loop

import "time"

type Cmd func() Msg

type Msg interface {
	isMsg() bool
}

type IsMsg struct{}

func (m IsMsg) isMsg() bool { return true }

type BatchMsg struct {
	IsMsg
	cmds []Cmd
}

func Batch(cmds ...Cmd) Cmd {
	var validCmds []Cmd
	for _, c := range cmds {
		if c == nil {
			continue
		}
		validCmds = append(validCmds, c)
	}
	if len(validCmds) == 0 {
		return nil
	}
	return func() Msg {
		return BatchMsg{
			cmds: validCmds,
		}
	}
}

type SequenceMsg struct {
	IsMsg
	cmds []Cmd
}

func Sequence(cmds ...Cmd) Cmd {
	return func() Msg {
		return SequenceMsg{
			cmds: cmds,
		}
	}
}

type Quitter interface {
	QuitError() error
}

type QuitMsg struct {
	IsMsg
	err error
}

func (q QuitMsg) QuitError() error {
	return q.err
}

func NewQuitMsg(err error) Msg {
	return &QuitMsg{err: err}
}

func Quit(err error) Cmd {
	return func() Msg {
		return QuitMsg{err: err}
	}
}

type TickMsg struct {
	IsMsg
	msg Msg
}

func Tick(delay time.Duration, msg Msg) Cmd {
	return func() Msg {
		time.Sleep(delay)
		return msg
	}
}
