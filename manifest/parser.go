package manifest

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
)

type ProtobufParser struct {
	parser *protoparse.Parser
}

func (p *ProtobufParser) Parse(files ...string) ([]*desc.FileDescriptor, error) {
	var fileDescriptors []*desc.FileDescriptor
	var err error

	for _, file := range files {
		if strings.Contains(file, "http") {
			tmp, err := p.parseFilesFromUrl(file)
			if err != nil {
				return nil, fmt.Errorf("parsing files from url: %w", err)
			}
			fileDescriptors = append(fileDescriptors, tmp...)
		} else {
			fileDescriptors, err = p.parser.ParseFiles(file)
			if err != nil {
				return nil, fmt.Errorf("parsing proto file: %w", err)
			}
		}
	}

	return fileDescriptors, nil
}

func (p *ProtobufParser) parseFilesFromUrl(fileURL string) ([]*desc.FileDescriptor, error) {
	filename := uuid.New().String() + ".proto"
	defer func() {
		err := os.Remove(filename)
		if err != nil {
			zlog.Error("failed to delete temporary proto file")
			return
		}
	}()

	resp, err := http.DefaultClient.Get(fileURL)
	if err != nil {
		return nil, fmt.Errorf("error downloading %q: %w", fileURL, err)
	}
	cnt, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %w", fileURL, err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("can't create temp file: %w", err)
	}
	_, err = io.Copy(file, bytes.NewBuffer(cnt))
	if err != nil {
		return nil, fmt.Errorf("can't write to temp file: %w", err)
	}

	return p.parser.ParseFiles(file.Name())
}
