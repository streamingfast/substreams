package loop

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventLoop_QuitMsg(t *testing.T) {
	tests := []struct {
		name        string
		initCmd     func(expectedErr error) Msg
		description string
	}{
		{
			name: "pointer_type",
			initCmd: func(expectedErr error) Msg {
				// Send a pointer QuitMsg using NewQuitMsg
				return NewQuitMsg(expectedErr)
			},
			description: "QuitMsg pointer (*QuitMsg) should be handled and exit loop",
		},
		{
			name: "value_type",
			initCmd: func(expectedErr error) Msg {
				// Send a value QuitMsg using Quit() wrapper
				return Quit(expectedErr)()
			},
			description: "QuitMsg value should be handled and exit loop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				expectedErr := errors.New("test error for " + tt.name)
				quitReceived := false

				updateFunc := func(msg Msg) Cmd {
					t.Logf("updateFunc received message: %T", msg)
					// Should not receive the quit message here since it's handled in the main loop
					return nil
				}

				loop := NewEventLoop(updateFunc)

				// Run the loop in a goroutine
				errChan := make(chan error, 1)
				go func() {
					err := loop.Run(ctx, func() Msg {
						t.Logf("Sending %s", tt.description)
						return tt.initCmd(expectedErr)
					})
					t.Logf("Loop exited with error: %v", err)
					quitReceived = true
					errChan <- err
				}()

				// Wait for the loop to exit
				select {
				case err := <-errChan:
					require.True(t, quitReceived, "Loop should have received quit message")
					assert.Equal(t, expectedErr, err, "Loop should exit with the expected error")
				case <-time.After(1 * time.Second):
					t.Fatalf("Loop did not exit within timeout - %s not handled!", tt.description)
				}
			})
		})
	}
}

func TestEventLoop_QuitMsg_FromCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		expectedErr := errors.New("error from command")
		messageReceived := false

		updateFunc := func(msg Msg) Cmd {
			t.Logf("updateFunc received message: %T", msg)

			// When we receive a custom message, return a command that will send a QuitMsg
			if _, ok := msg.(customMsg); ok {
				messageReceived = true
				return func() Msg {
					t.Log("Command returning NewQuitMsg()")
					return NewQuitMsg(expectedErr)
				}
			}
			return nil
		}

		loop := NewEventLoop(updateFunc)

		// Run the loop in a goroutine
		errChan := make(chan error, 1)
		go func() {
			err := loop.Run(ctx, func() Msg {
				// Send a custom message that will trigger a command returning QuitMsg
				t.Log("Sending custom message")
				return customMsg{}
			})
			t.Logf("Loop exited with error: %v", err)
			errChan <- err
		}()

		// Wait for the loop to exit
		select {
		case err := <-errChan:
			require.True(t, messageReceived, "Custom message should have been received")
			assert.Equal(t, expectedErr, err, "Loop should exit with error from command")
		case <-time.After(1 * time.Second):
			t.Fatal("Loop did not exit within timeout - QuitMsg from command not handled!")
		}
	})
}

func TestEventLoop_ContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		updateFunc := func(msg Msg) Cmd {
			t.Logf("updateFunc received message: %T", msg)
			// Keep the loop running by returning nil
			return nil
		}

		loop := NewEventLoop(updateFunc)

		// Run the loop in a goroutine
		errChan := make(chan error, 1)
		go func() {
			err := loop.Run(ctx, func() Msg {
				// Send a message to keep the loop running
				t.Log("Sending custom message to keep loop running")
				return customMsg{}
			})
			t.Logf("Loop exited with error: %v", err)
			errChan <- err
		}()

		// Give the loop time to start
		time.Sleep(100 * time.Millisecond)

		// Cancel the context
		t.Log("Cancelling context")
		cancel()

		// Wait for the loop to exit
		select {
		case err := <-errChan:
			assert.Equal(t, context.Canceled, err, "Loop should exit with context.Canceled")
		case <-time.After(1 * time.Second):
			t.Fatal("Loop did not exit within timeout after context cancellation!")
		}
	})
}

func TestEventLoop_BatchMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		messagesReceived := 0
		expectedErr := errors.New("batch quit error")

		updateFunc := func(msg Msg) Cmd {
			t.Logf("updateFunc received message: %T", msg)

			if _, ok := msg.(customMsg); ok {
				messagesReceived++
				t.Logf("Received custom message %d", messagesReceived)

				// After receiving 3 messages, send a batch that includes a quit
				if messagesReceived == 3 {
					return Batch(
						func() Msg { return customMsg{} },
						func() Msg { return NewQuitMsg(expectedErr) },
					)
				}
			}
			return nil
		}

		loop := NewEventLoop(updateFunc)

		// Run the loop in a goroutine
		errChan := make(chan error, 1)
		go func() {
			err := loop.Run(ctx, func() Msg {
				// Start by sending messages - Batch returns a Cmd, so call it to get the Msg
				return Batch(
					func() Msg { return customMsg{} },
					func() Msg { return customMsg{} },
					func() Msg { return customMsg{} },
				)()
			})
			t.Logf("Loop exited with error: %v", err)
			errChan <- err
		}()

		// Wait for the loop to exit
		select {
		case err := <-errChan:
			assert.Equal(t, expectedErr, err, "Loop should exit with error from batch")
			assert.GreaterOrEqual(t, messagesReceived, 3, "Should have received at least 3 messages")
		case <-time.After(1 * time.Second):
			t.Fatal("Loop did not exit within timeout - batch QuitMsg not handled!")
		}
	})
}

// customMsg is a test message type
type customMsg struct {
	IsMsg
}
