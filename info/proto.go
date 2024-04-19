package info

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoprint"
	"google.golang.org/protobuf/types/descriptorpb"
)

type ProtoPackageParser struct {
	allFiles            []*descriptorpb.FileDescriptorProto
	nestedMessagesAdded map[string]bool

	fileCodeMap    map[string]string
	filePacakgeMap map[string]string
}

func NewProtoPackageParser(files []*descriptorpb.FileDescriptorProto) (*ProtoPackageParser, error) {
	p := &ProtoPackageParser{
		allFiles:            files,
		nestedMessagesAdded: make(map[string]bool),
	}

	desc, err := desc.CreateFileDescriptors(p.allFiles)
	if err != nil {
		return nil, err
	}

	printer := &protoprint.Printer{
		Compact: true,
	}
	fileCodeMap := make(map[string]string)
	filePackageMap := make(map[string]string)
	for fd, d := range desc {
		r, err := printer.PrintProtoToString(d)
		if err != nil {
			return nil, err
		}
		fileCodeMap[fd] = r
		filePackageMap[fd] = d.GetPackage()
	}
	p.fileCodeMap = fileCodeMap
	p.filePacakgeMap = filePackageMap

	return p, nil
}

func (p *ProtoPackageParser) Parse() (map[string][]*ProtoMessageInfo, error) {
	result := map[string][]*ProtoMessageInfo{}

	for _, file := range p.allFiles {
		result[file.GetPackage()] = append(result[file.GetPackage()], p.extractMessages(file, "", file.MessageType)...)

		for _, enum := range file.GetEnumType() {
			doc := getDocumentationForSymbol(file.GetSourceCodeInfo(), enum.GetName())
			protoCode, err := extractEnumBlock(p.fileCodeMap[file.GetName()], enum.GetName())
			if err != nil {
				return nil, fmt.Errorf("extract message block: %w", err)
			}
			result[file.GetPackage()] = append(result[file.GetPackage()], &ProtoMessageInfo{
				Name:          enum.GetName(),
				Package:       file.GetPackage(),
				Type:          "enum",
				File:          file.GetName(),
				Proto:         protoCode,
				Documentation: doc,
			})
		}

	}

	return result, nil
}

func (p *ProtoPackageParser) extractMessages(file *descriptorpb.FileDescriptorProto, prefix string, messages []*descriptorpb.DescriptorProto) []*ProtoMessageInfo {
	var results []*ProtoMessageInfo

	for _, msg := range messages {
		doc := getDocumentationForSymbol(file.GetSourceCodeInfo(), msg.GetName())
		protoCode, err := extractMessageBlock(p.fileCodeMap[file.GetName()], msg.GetName())
		if err != nil {
			return nil
		}

		name := prefix + msg.GetName()
		result := &ProtoMessageInfo{
			Name:          name,
			Package:       file.GetPackage(),
			Type:          "Message",
			File:          file.GetName(),
			Proto:         protoCode,
			Documentation: doc,
		}
		if len(msg.GetNestedType()) > 0 {
			result.NestedMessages = append(result.NestedMessages, p.extractMessages(file, name+".", msg.GetNestedType())...)
		}
		if len(msg.GetEnumType()) > 0 {
			result.NestedMessages = append(result.NestedMessages, p.extractEnums(file, name+".", msg.GetEnumType())...)
		}
		results = append(results, result)
	}

	return results
}

func (p *ProtoPackageParser) extractEnums(file *descriptorpb.FileDescriptorProto, prefix string, enums []*descriptorpb.EnumDescriptorProto) []*ProtoMessageInfo {
	var results []*ProtoMessageInfo

	for _, enum := range enums {
		doc := getDocumentationForSymbol(file.GetSourceCodeInfo(), enum.GetName())
		protoCode, err := extractEnumBlock(p.fileCodeMap[file.GetName()], enum.GetName())
		if err != nil {
			return nil
		}

		name := prefix + enum.GetName()
		results = append(results, &ProtoMessageInfo{
			Name:          name,
			Package:       file.GetPackage(),
			Type:          "Enum",
			File:          file.GetName(),
			Proto:         protoCode,
			Documentation: doc,
		})
	}

	return results
}

func (p *ProtoPackageParser) GetPackagesList() []string {
	packages := make(map[string]bool)
	for _, file := range p.allFiles {
		packages[file.GetPackage()] = true
	}

	var result []string
	for pkg := range packages {
		result = append(result, pkg)
	}

	return result
}

func (p *ProtoPackageParser) GetFilesSourceCode() map[string][]*SourceCodeInfo {
	result := make(map[string][]*SourceCodeInfo)
	for filename, pkg := range p.filePacakgeMap {
		source := p.fileCodeMap[filename]
		result[pkg] = append(result[pkg], &SourceCodeInfo{
			Filename: filename,
			Source:   source,
		})
	}

	return result
}

// getDocumentationForSymbol extracts the leading comments associated with a named symbol (message/enum)
func getDocumentationForSymbol(sourceInfo *descriptorpb.SourceCodeInfo, symbolName string) string {
	for _, location := range sourceInfo.GetLocation() {
		if strings.HasPrefix(strings.TrimSpace(location.GetLeadingComments()), symbolName) {
			return strings.TrimSpace(location.GetLeadingComments())
		}
	}
	return ""
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

func extractEnumBlock(protoContent, messageName string) (string, error) {
	pattern := fmt.Sprintf(`(?s)enum\s+%s\s+\{.*?\}`, messageName)
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(protoContent)
	if matches == nil {
		return "", fmt.Errorf("no message block found for enum %q", messageName)
	}

	return matches[0], nil
}
