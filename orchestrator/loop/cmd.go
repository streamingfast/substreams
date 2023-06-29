package loop

type Msg any

type Cmd func() Msg

type BatchMsg []Cmd

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
		return BatchMsg(validCmds)
	}
}

type SequenceMsg []Cmd

func Sequence(cmds ...Cmd) Cmd {
	return func() Msg {
		return SequenceMsg(cmds)
	}
}

type QuitMsg struct {
	err error
}

func Quit(err error) Cmd {
	return func() Msg {
		return QuitMsg{err}
	}
}
