package sink

import (
	"os"
	"strings"
)

// ReadCursor will return nil if filename is empty or not found
func ReadCursor(filename string) (*Cursor, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	cursorStr := strings.TrimSpace(string(data))
	if cursorStr != "" {
		return NewCursor(cursorStr)
	}
	return nil, nil
}

// WriteCursor is a NOOP if filename is empty or cursor is nil
func WriteCursor(filename string, cursor *Cursor) error {
	if filename == "" || cursor == nil {
		return nil
	}
	return os.WriteFile(filename, []byte(cursor.String()), 0644)
}
