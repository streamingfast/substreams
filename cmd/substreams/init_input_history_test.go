package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputHistory_Add(t *testing.T) {
	history := NewInputHistory()

	history.Add("first")
	history.Add("second")
	history.Add("second") // Duplicate should not be added
	history.Add("")       // Empty should not be added

	assert.Len(t, history.entries, 2)
	assert.Equal(t, "first", history.entries[0])
	assert.Equal(t, "second", history.entries[1])
}

func TestInputHistory_NavigateUp(t *testing.T) {
	history := NewInputHistory()
	history.Add("first")
	history.Add("second")
	history.Add("third")

	// First up should return most recent
	value, changed := history.NavigateUp("current")
	require.True(t, changed)
	assert.Equal(t, "third", value)

	// Second up should return previous
	value, changed = history.NavigateUp("current")
	require.True(t, changed)
	assert.Equal(t, "second", value)

	// Third up should return first
	value, changed = history.NavigateUp("current")
	require.True(t, changed)
	assert.Equal(t, "first", value)

	// Fourth up should stay at first
	value, changed = history.NavigateUp("current")
	assert.False(t, changed)
	assert.Equal(t, "first", value)
}

func TestInputHistory_NavigateDown(t *testing.T) {
	history := NewInputHistory()
	history.Add("first")
	history.Add("second")
	history.Add("third")

	// Navigate up to start
	history.NavigateUp("current")
	history.NavigateUp("current")
	history.NavigateUp("current")

	// Now navigate down
	value, changed := history.NavigateDown("current")
	require.True(t, changed)
	assert.Equal(t, "second", value)

	value, changed = history.NavigateDown("current")
	require.True(t, changed)
	assert.Equal(t, "third", value)

	// At the end, should return original current value
	value, changed = history.NavigateDown("current")
	require.True(t, changed)
	assert.Equal(t, "current", value)

	// Further down should not change
	value, changed = history.NavigateDown("current")
	assert.False(t, changed)
}

func TestInputHistory_Empty(t *testing.T) {
	history := NewInputHistory()

	value, changed := history.NavigateUp("test")
	assert.False(t, changed)
	assert.Equal(t, "test", value)
}

func TestInputHistory_Reset(t *testing.T) {
	history := NewInputHistory()
	history.Add("first")
	history.Add("second")

	// Navigate up to set cursor
	history.NavigateUp("current")
	assert.Equal(t, 1, history.cursor)
	assert.Equal(t, "current", history.current)

	// Reset should clear navigation state
	history.Reset()
	assert.Equal(t, -1, history.cursor)
	assert.Equal(t, "", history.current)
}

func TestInputHistory_ThreadSafety(t *testing.T) {
	history := NewInputHistory()

	// Test concurrent access
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			history.Add("test")
			history.NavigateUp("current")
			history.NavigateDown("current")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			history.Add("test2")
			history.NavigateUp("current2")
			history.NavigateDown("current2")
		}
		done <- true
	}()

	<-done
	<-done

	// Should not panic and should have some entries
	assert.True(t, len(history.entries) > 0)
}
