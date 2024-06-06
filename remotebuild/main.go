package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/exp/slog"

	"connectrpc.com/connect"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/lithammer/dedent"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	pbbuild "github.com/streamingfast/substreams/remotebuild/pb/sf/remotebuild/v1"
	"github.com/streamingfast/substreams/remotebuild/pb/sf/remotebuild/v1/pbbuildv1connect"
)

type Server struct {
	logger *slog.Logger
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	srv := &Server{
		logger: logger,
	}

	mux := http.NewServeMux()
	path, handler := pbbuildv1connect.NewBuildServiceHandler(srv)
	mux.Handle(path, handler)

	port := "9000"
	srv.logger.Info("listening on port", "port", port)
	err := http.ListenAndServe(
		fmt.Sprintf(":%s", port),
		// Use h2c so we can serve HTTP/2 without TLS.
		h2c.NewHandler(mux, &http2.Server{
			MaxHandlers:          100,
			MaxConcurrentStreams: 250,
			IdleTimeout:          5 * time.Minute,
			MaxReadFrameSize:     1048576, // 1MB
		}),
	)

	if err != nil && err != http.ErrServerClosed {
		srv.logger.Error("server listen error", zap.Error(err))
	}
}

func (s *Server) Build(
	ctx context.Context,
	req *connect.Request[pbbuild.BuildRequest],
	stream *connect.ServerStream[pbbuild.BuildResponse],
) error {
	tempDir, err := os.MkdirTemp(os.TempDir(), "remotebuild")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	if os.Getenv("GENERATOR_KEEP_FILES") != "true" {
		defer func() {
			err := os.RemoveAll(tempDir)
			if err != nil {
				s.logger.Warn("failed to remove temp dir", zap.Error(err))
			}
		}()
	} else {
		s.logger.Info("keeping temp dir", "dir", tempDir)
	}
	s.logger.Debug("temp dir", "dir", tempDir)
	s.logger.Debug("source code size", "size", len(req.Msg.SourceCode))
	err = unzip(req.Msg.SourceCode, tempDir)
	if err != nil {
		return fmt.Errorf("unzipping: %w", err)
	}

	folders, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("reading temp dir: %w", err)
	}

	if len(folders) != 1 {
		return fmt.Errorf("expected exactly one folder in temp dir, got %d", len(folders))
	}

	// here inside the folder, it will contain the name of the folder that the files were unzipped into
	workingDir := filepath.Join(tempDir, folders[0].Name())

	// Check to add this in the docker file and make it work as expected
	content := dedent.Dedent(`
		make package
	`)

	// here we need to write a run.sh script to allow /bin/sh to run multiple commands
	// from the Makefile. If not, it will always only run the first command (make protogen)
	if err := os.WriteFile(filepath.Join(workingDir, "run.sh"), []byte(content), 0755); err != nil {
		return fmt.Errorf("writing run.sh: %w", err)
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "./run.sh")

	// Setup environmental variables for the command run
	// Also add in any environmental variables passed in the request
	cmd.Env = append(req.Msg.Env, os.Environ()...)

	cmd.Dir = workingDir

	stdoutBuf := &bytes.Buffer{}
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, stdoutBuf)

	progressLogsOffset := 0

	ctx, cancelProgressSender := context.WithCancel(ctx)
	go func() {
		timer := time.NewTicker(1 * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				to := len(stdoutBuf.Bytes())
				progressLogs := stdoutBuf.Bytes()[progressLogsOffset:to]
				// todo keep sending progress logs
				stream.Send(&pbbuild.BuildResponse{
					Logs: string(progressLogs),
				})
				progressLogsOffset = to
			case <-ctx.Done():
				return
			}
		}
	}()

	err = cmd.Run()
	cancelProgressSender()

	if err != nil {
		stream.Send(&pbbuild.BuildResponse{
			Error: err.Error(),
			// send the rest of the logs that have not been sent yet
			Logs: string(stdoutBuf.Bytes()[progressLogsOffset:]),
		})
		return fmt.Errorf("running build command: %w", err)
	}

	artifacts, err := s.collectArtifacts(workingDir, "substreams.spkg")
	if err != nil {
		stream.Send(&pbbuild.BuildResponse{
			Error: err.Error(),
			// no logs to send here as we have an error which is unreleated to the build
		})
		return fmt.Errorf("collecting artifacts: %w", err)
	}

	stream.Send(&pbbuild.BuildResponse{
		Artifacts: artifacts,
		// send the rest of the logs that have not been sent yet
		Logs: string(stdoutBuf.Bytes()[progressLogsOffset:]),
	})

	return nil
}

func (s *Server) collectArtifacts(dir string, pattern string) (out []*pbbuild.BuildResponse_BuildArtifact, err error) {
	// Currently pattern will always be substreams.spkg
	matching, err := doublestar.Glob(os.DirFS(dir), pattern)
	if err != nil {
		return nil, fmt.Errorf("reading output dir: %w", err)
	}

	for _, file := range matching {
		content, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", filepath.Join(dir, file), err)
		}
		out = append(out, &pbbuild.BuildResponse_BuildArtifact{
			Filename: file,
			Content:  content,
		})
	}

	return
}

func unzip(sourceBytes []byte, dest string) error {
	reader := bytes.NewReader(sourceBytes)
	r, err := zip.NewReader(reader, int64(len(sourceBytes)))
	if err != nil {
		return err
	}

	err = os.MkdirAll(dest, 0755)
	if err != nil {
		return err
	}

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}
