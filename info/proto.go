package info

import (
	"fmt"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoprint"
	"google.golang.org/protobuf/types/descriptorpb"
	"regexp"
	"strings"
)

type ProtoParser struct {
	fileDescriptors      map[string]*desc.FileDescriptor
	fileDescriptorModule map[string]string

	ProtoFileCodeMap           map[string]string
	ProtoPackageMessageCodeMap map[string]map[string]string

	protoFiles []*descriptorpb.FileDescriptorProto
}

func NewProtoParser(protoFiles []*descriptorpb.FileDescriptorProto) (*ProtoParser, error) {
	desc, err := desc.CreateFileDescriptors(protoFiles)
	if err != nil {
		return nil, err
	}

	fdm := make(map[string]string)
	for fd := range desc {
		res := strings.Split(fd, "/")
		if len(res) == 0 {
			continue
		}
		if strings.HasSuffix(res[len(res)-1], ".proto") {
			res = res[:len(res)-1]
		}
		fdm[fd] = strings.Join(res, ".")
	}

	return &ProtoParser{
		fileDescriptors:      desc,
		fileDescriptorModule: fdm,
		protoFiles:           protoFiles,
	}, nil
}

func (p *ProtoParser) Parse() error {
	err := p.parseFiles()
	if err != nil {
		return fmt.Errorf("parse files: %w", err)
	}
	err = p.parseMessages()
	if err != nil {
		return fmt.Errorf("parse messages: %w", err)
	}

	return nil
}

func (p *ProtoParser) parseFiles() error {
	printer := &protoprint.Printer{
		Compact: true,
	}
	res := make(map[string]string)
	for fd, d := range p.fileDescriptors {
		r, err := printer.PrintProtoToString(d)
		if err != nil {
			return err
		}
		res[fd] = r
	}

	p.ProtoFileCodeMap = res
	return nil
}

func (p *ProtoParser) parseMessages() error {
	msgCodeMap := make(map[string]map[string]string)
	for _, protoFile := range p.protoFiles {
		if _, ok := msgCodeMap[protoFile.GetPackage()]; !ok {
			msgCodeMap[protoFile.GetPackage()] = make(map[string]string)
		}

		for _, msg := range protoFile.GetMessageType() {
			msgCode, err := extractMessageBlock(p.ProtoFileCodeMap[protoFile.GetName()], msg.GetName())
			if err != nil {
				return fmt.Errorf("extract message block: %w", err)
			}
			msgCodeMap[protoFile.GetPackage()][msg.GetName()] = msgCode
		}

		for _, enum := range protoFile.GetEnumType() {
			msgCode, err := extractMessageBlock(p.ProtoFileCodeMap[protoFile.GetName()], enum.GetName())
			if err != nil {
				return fmt.Errorf("extract message block: %w", err)
			}
			msgCodeMap[protoFile.GetPackage()][enum.GetName()] = msgCode
		}
	}

	p.ProtoPackageMessageCodeMap = msgCodeMap
	return nil
}

func extractMessageBlock(protoContent, messageName string) (string, error) {
	pattern := fmt.Sprintf(`(?s)message\s+%s\s+\{.*?\}`, messageName)
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(protoContent)
	if matches == nil {
		return "", fmt.Errorf("no message block found for message %q", messageName)
	}

	return matches[0], nil
}
