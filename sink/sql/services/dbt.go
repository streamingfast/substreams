package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	pbsql "github.com/streamingfast/substreams/sink/sql/pb/sf/substreams/sink/sql/services/v1"
	"go.uber.org/zap"
)

func runDBT(config *pbsql.DBTConfig, logger *zap.Logger) error {
	data := config.Files
	dbtDir := "/tmp/dbt"

	if err := os.RemoveAll(dbtDir); err != nil {
		return fmt.Errorf("removing dbt directory: %w", err)
	}

	if err := os.MkdirAll(dbtDir, os.ModePerm); err != nil {
		return fmt.Errorf("creating dbt directory: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("reading zip data from config: %w", err)
	}

	// Extract each file in the archive
	for _, file := range reader.File {
		filePath := filepath.Join(dbtDir, file.Name)

		// Ensure the file's directory structure exists
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return fmt.Errorf("creating directory %s: %w", file.FileInfo().Name(), err)
			}
			continue
		}

		// Ensure parent directories exist
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return fmt.Errorf("creating parent directory %s: %w", filePath, err)
		}

		// Open the file inside the zip
		srcFile, err := file.Open()
		if err != nil {
			return fmt.Errorf("opening file %s: %w", file.FileInfo().Name(), err)
		}
		defer srcFile.Close()

		// Create the destination file
		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return fmt.Errorf("creating file %s: %w", filePath, err)
		}
		defer destFile.Close()

		// Copy the file contents
		if _, err := io.Copy(destFile, srcFile); err != nil {
			return fmt.Errorf("copying file %s: %w", filePath, err)
		}
	}

	for {
		logger.Info("running dbt")
		cmd := exec.Command("dbt", "run", "--profiles-dir", "/tmp/dbt", "--project-dir", "/tmp/dbt")
		cmd.Env = os.Environ()
		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Error("running dbt", zap.Error(err), zap.ByteString("output", output))
			return fmt.Errorf("running dbt: %w", err)
		}
		logger.Info("dbt output")
		fmt.Println(string(output))

		time.Sleep(time.Duration(config.RunIntervalSeconds))
	}
}
