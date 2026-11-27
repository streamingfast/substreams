package sink

import (
	"os"
	"path/filepath"
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

// WriteCursor writes cursor to file using temp file and rename. Returns nil if filename is empty or cursor is nil.
func WriteCursor(filename string, cursor *Cursor) error {
	if filename == "" || cursor == nil {
		return nil
	}

	return WriteFileAtomic(filename, []byte(cursor.String()))
}

// WriteFileAtomic writes data to a temp file next to filename and renames it over filename, so readers
// never see a partially written file. The data is synced to disk before the rename.
func WriteFileAtomic(filename string, data []byte) error {
	tempFile, err := os.CreateTemp(filepath.Dir(filename), ".tmp_*")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()

	// cleanup, prevents leaving temp file on disk if rename fails
	defer os.Remove(tempPath)

	if _, err = tempFile.Write(data); err != nil {
		tempFile.Close()
		return err
	}

	if err = tempFile.Sync(); err != nil {
		tempFile.Close()
		return err
	}

	if err = tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, filename)
}
