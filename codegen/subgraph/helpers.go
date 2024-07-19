package subgraph

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

func GetProjectEntities(outputDescriptor *descriptorpb.DescriptorProto, protoTypeMapping map[string]*descriptorpb.DescriptorProto) (map[string]Entity, error) {
	var outputMap = map[string]Entity{}
	err := buildEntitiesMapping(outputDescriptor, outputMap, protoTypeMapping)
	if err != nil {
		return nil, fmt.Errorf("getting entities: %w", err)
	}

	return outputMap, nil
}

type Entity struct {
	NestedEntitiesMapping map[string]string
	protoMessage          *descriptorpb.DescriptorProto
	protoPath             string
	ProtobufPath          string
	HasClassicTypes       bool
	NameAsProto           string
	NameAsEntity          string
}

func getMessageProtoPath(message *descriptorpb.DescriptorProto, protoTypeMapping map[string]*descriptorpb.DescriptorProto) (string, error) {
	for protoPath, currentMessage := range protoTypeMapping {
		if currentMessage == message {
			return protoPath, nil
		}
	}

	return "", fmt.Errorf("proto path not found for message %q", message.Name)
}

func buildEntitiesMapping(message *descriptorpb.DescriptorProto, inputMap map[string]Entity, protoTypeMapping map[string]*descriptorpb.DescriptorProto) error {
	protoPath, err := getMessageProtoPath(message, protoTypeMapping)
	if err != nil {
		return fmt.Errorf("getting proto path: %w", err)
	}

	protobufPath, _ := strings.CutPrefix(protoPath, ".")
	protobufPath = strings.ReplaceAll(protobufPath, ".", "/")

	var entity = Entity{
		protoMessage:          message,
		protoPath:             protoPath,
		ProtobufPath:          protobufPath,
		NestedEntitiesMapping: map[string]string{},
		HasClassicTypes:       false,
		NameAsProto:           "proto" + message.GetName(),
		NameAsEntity:          "entity" + message.GetName(),
	}

	for _, field := range message.GetField() {
		switch *field.Type {
		case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
			sanitizeFieldName := (*field.TypeName)[strings.LastIndex(*field.TypeName, ".")+1:]
			switch *field.Label {
			case descriptorpb.FieldDescriptorProto_LABEL_REPEATED:
				entity.NestedEntitiesMapping[field.GetName()] = "[" + sanitizeFieldName + "!]!"
			case descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL:
				entity.NestedEntitiesMapping[field.GetName()] = sanitizeFieldName + "!"
			case descriptorpb.FieldDescriptorProto_LABEL_REQUIRED:
				entity.NestedEntitiesMapping[field.GetName()] = sanitizeFieldName + "!"
			default:
				return fmt.Errorf("field label %q not supported", *field.Label)
			}
			nestedMessage := protoTypeMapping[*field.TypeName]
			err := buildEntitiesMapping(nestedMessage, inputMap, protoTypeMapping)
			if err != nil {
				return fmt.Errorf("getting entity from message: %w", err)
			}
		case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
			return fmt.Errorf("enum type not supported")
		default:
			entity.HasClassicTypes = true
		}
	}

	entityName := message.GetName()
	inputMap[entityName] = entity
	return nil
}

func GetExistingProtoTypes(protoFiles []*descriptorpb.FileDescriptorProto) map[string]*descriptorpb.DescriptorProto {
	var protoTypeMapping = map[string]*descriptorpb.DescriptorProto{}
	for _, protoFile := range protoFiles {
		packageName := protoFile.GetPackage()
		for _, message := range protoFile.MessageType {
			currentName := "." + packageName + "." + message.GetName()
			protoTypeMapping[currentName] = message
			processMessage(message, currentName, protoTypeMapping)
		}
	}

	return protoTypeMapping
}

func processMessage(message *descriptorpb.DescriptorProto, parentName string, protoTypeMapping map[string]*descriptorpb.DescriptorProto) {
	for _, nestedMessage := range message.NestedType {
		currentName := "." + parentName + "." + nestedMessage.GetName()
		protoTypeMapping[currentName] = nestedMessage
		processMessage(nestedMessage, currentName, protoTypeMapping)
	}
}

func GetModule(pkg *pbsubstreams.Package, moduleName string) (*pbsubstreams.Module, error) {
	existingModules := pkg.GetModules().GetModules()
	for _, module := range existingModules {
		if (module.Name) == moduleName {
			return module, nil
		}
	}

	return nil, fmt.Errorf("module %q does not exists", moduleName)
}

func SearchForMessageTypeIntoPackage(pkg *pbsubstreams.Package, outputType string) (*descriptorpb.DescriptorProto, error) {
	sanitizeMessageType := outputType[strings.Index(outputType, ":")+1:]
	for _, protoFile := range pkg.ProtoFiles {
		packageName := protoFile.GetPackage()
		for _, message := range protoFile.MessageType {
			if packageName+"."+message.GetName() == sanitizeMessageType {
				return message, nil
			}

			nestedMessage := checkNestedMessages(message, packageName, sanitizeMessageType)
			if nestedMessage != nil {
				return nestedMessage, nil
			}
		}
	}

	return nil, fmt.Errorf("message type %q not found in package", sanitizeMessageType)
}

func checkNestedMessages(message *descriptorpb.DescriptorProto, packageName, messageType string) *descriptorpb.DescriptorProto {
	for _, nestedMessage := range message.NestedType {
		if packageName+"."+message.GetName()+"."+nestedMessage.GetName() == messageType {
			return nestedMessage
		}

		checkNestedMessages(nestedMessage, packageName, messageType)
	}

	return nil
}
