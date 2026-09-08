package proto

import (
	"fmt"

	schema "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TableInfo(d protoreflect.MessageDescriptor) *schema.Table {
	msgOptions := d.Options()

	if proto.HasExtension(msgOptions, schema.E_Table) {
		ext := proto.GetExtension(msgOptions, schema.E_Table)
		table, ok := ext.(*schema.Table)
		if ok {
			if table.Name == "" {
				panic(fmt.Sprintf("table name is required for message %q", string(d.Name())))
			}
			return table
		}
	}
	return nil
}

func FieldInfo(d protoreflect.FieldDescriptor) *schema.Column {
	options := d.Options()

	if proto.HasExtension(options, schema.E_Field) {
		ext := proto.GetExtension(options, schema.E_Field)
		f, ok := ext.(*schema.Column)
		if ok {
			return f
		}
	}
	return nil
}
