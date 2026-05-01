package sql

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func fieldName(f protoreflect.FieldDescriptor) string {
	fieldNameSuffix := ""
	if f.Kind() == protoreflect.MessageKind {
		fieldNameSuffix = "_id"
	}

	return fmt.Sprintf("%s%s", strings.ToLower(string(f.Name())), fieldNameSuffix)
}

func fieldQuotedName(f protoreflect.FieldDescriptor) string {
	return Quoted(fieldName(f))
}

func Quoted(value string) string {
	return fmt.Sprintf("\"%s\"", value)
}
