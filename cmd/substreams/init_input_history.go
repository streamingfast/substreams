package main

import (
	"sync"
)

// InputHistory manages a session-based history of text inputs
type InputHistory struct {
	mu      sync.RWMutex
	entries []string
	cursor  int    // Current position in history (-1 means not navigating)
	current string // Stores the text user was typing before navigating history
}

// NewInputHistory creates a new input history tracker
func NewInputHistory() *InputHistory {
	return &InputHistory{
		entries: make([]string, 0, 50), // Pre-allocate for efficiency
		cursor:  -1,
	}
}

// Add records a new input to history (called after user submits input)
func (h *InputHistory) Add(input string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Don't add empty inputs or duplicates of the last entry
	if input == "" || (len(h.entries) > 0 && h.entries[len(h.entries)-1] == input) {
		return
	}

	h.entries = append(h.entries, input)
	h.cursor = -1 // Reset navigation state
}

// NavigateUp returns the previous entry in history
// Returns (value, changed) where changed indicates if navigation occurred
func (h *InputHistory) NavigateUp(currentValue string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.entries) == 0 {
		return currentValue, false
	}

	// First time navigating, save current input
	if h.cursor == -1 {
		h.current = currentValue
		h.cursor = len(h.entries) - 1
		return h.entries[h.cursor], true
	}

	// Already navigating, go further back
	if h.cursor > 0 {
		h.cursor--
		return h.entries[h.cursor], true
	}

	// At the oldest entry, no change
	return h.entries[h.cursor], false
}

// NavigateDown returns the next entry in history
// Returns (value, changed) where changed indicates if navigation occurred
func (h *InputHistory) NavigateDown(currentValue string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cursor == -1 {
		// Not currently navigating history
		return currentValue, false
	}

	if h.cursor < len(h.entries)-1 {
		h.cursor++
		return h.entries[h.cursor], true
	}

	// Reached the end, return to user's current input
	h.cursor = -1
	return h.current, true
}

// Reset clears navigation state (call when input is submitted or cancelled)
func (h *InputHistory) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cursor = -1
	h.current = ""
}
