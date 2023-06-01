package loop

import "context"

// loop is the micro framework for the Scheduler's event loop,
// heavily inspired by BubbleTea, which we use for the substreams GUI.

type Msg any

type Cmd func() Msg

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

type BatchMsg []Cmd

func Sequence(cmds ...Cmd) Cmd {
	return func() Msg {
		return SequenceMsg(cmds)
	}
}

type SequenceMsg []Cmd

type EventLoop struct {
	ctx        context.Context
	msgs       chan Msg
	updateFunc func(msg Msg) Cmd
}

func NewEventLoop(ctx context.Context, updateFunc func(msg Msg) Cmd) *EventLoop {
	return &EventLoop{
		ctx:        ctx,
		msgs:       make(chan Msg, 1000),
		updateFunc: updateFunc,
	}
}

func (l *EventLoop) Run() (err error) {
	cmds := make(chan loop.Cmd, 1000)
	// main execution loop
	done := l.handleCommands(cmds)
loop:
	for {
		select {
		case <-l.ctx.Done():
			err = l.ctx.Err()
			break loop
		case msg := <-l.msgs:
			cmd := l.update(msg, cmds)
			if cmd == nil {
				continue
			}
			go func() {
				l.Send(cmd())
			}()
		}
	}
	<-done
	return
}

func (l *EventLoop) Send(msg Msg) {
	select {
	case <-l.ctx.Done():
	case l.msgs <- msg:
	}
}

func (l *EventLoop) update(msg Msg, cmds chan Cmd) Cmd {
	switch msg := msg.(type) {
	case BatchMsg:
		for _, cmd := range msg {
			cmds <- cmd
		}
		return nil

	case SequenceMsg:
		go func() {
			// Execute commands one at a time, in order.
			for _, cmd := range msg {
				l.Send(cmd())
			}
		}()
	}

	return l.updateFunc(msg)
}

func (l *EventLoop) handleCommands(cmds chan Cmd) chan struct{} {
	ch := make(chan struct{})

	go func() {
		defer close(ch)

		for {
			select {
			case <-l.ctx.Done():
				return

			case cmd := <-cmds:
				if cmd == nil {
					continue
				}

				go func() {
					msg := cmd() // this can be long.
					l.Send(msg)
				}()
			}
		}
	}()

	return ch
}
