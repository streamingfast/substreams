// Code generated stub for compilation purposes only.
package pbdatabase

import "google.golang.org/protobuf/reflect/protoreflect"

type TableChange_Operation int32

const (
	TableChange_OPERATION_UNSET  TableChange_Operation = 0
	TableChange_OPERATION_CREATE TableChange_Operation = 1
	TableChange_OPERATION_UPDATE TableChange_Operation = 2
	TableChange_OPERATION_DELETE TableChange_Operation = 3
	TableChange_OPERATION_UPSERT TableChange_Operation = 4
)

type Field_UpdateOp int32

const (
	Field_UPDATE_OP_UNSET      Field_UpdateOp = 0
	Field_UPDATE_OP_ADD        Field_UpdateOp = 1
	Field_UPDATE_OP_MAX        Field_UpdateOp = 2
	Field_UPDATE_OP_MIN        Field_UpdateOp = 3
	Field_UPDATE_OP_SET_IF_NULL Field_UpdateOp = 4
)

type DatabaseChanges struct {
	TableChanges []*TableChange `protobuf:"bytes,1,rep,name=table_changes,json=tableChanges,proto3" json:"table_changes,omitempty"`
}

func (x *DatabaseChanges) Reset()                        {}
func (x *DatabaseChanges) String() string                { return "" }
func (x *DatabaseChanges) ProtoMessage()                 {}
func (x *DatabaseChanges) ProtoReflect() protoreflect.Message { return nil }

type isTableChange_PrimaryKey interface {
	isTableChange_PrimaryKey()
}

type TableChange_Pk struct {
	Pk string
}

func (*TableChange_Pk) isTableChange_PrimaryKey() {}

type TableChange_CompositePk struct {
	CompositePk *CompositePrimaryKey
}

func (*TableChange_CompositePk) isTableChange_PrimaryKey() {}

type TableChange struct {
	Table      string                `protobuf:"bytes,1,opt,name=table,proto3" json:"table,omitempty"`
	Operation  TableChange_Operation `protobuf:"varint,2,opt,name=operation,proto3,enum=sf.substreams.sink.database.v1.TableChange_Operation" json:"operation,omitempty"`
	Fields     []*Field              `protobuf:"bytes,4,rep,name=fields,proto3" json:"fields,omitempty"`
	PrimaryKey isTableChange_PrimaryKey
}

func (x *TableChange) Reset()                        {}
func (x *TableChange) String() string                { return x.Table }
func (x *TableChange) ProtoMessage()                 {}
func (x *TableChange) ProtoReflect() protoreflect.Message { return nil }

type CompositePrimaryKey struct {
	Keys map[string]string `protobuf:"bytes,1,rep,name=keys,proto3" json:"keys,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *CompositePrimaryKey) Reset()                        {}
func (x *CompositePrimaryKey) String() string                { return "" }
func (x *CompositePrimaryKey) ProtoMessage()                 {}
func (x *CompositePrimaryKey) ProtoReflect() protoreflect.Message { return nil }

type Field struct {
	Name     string         `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Value    string         `protobuf:"bytes,3,opt,name=new_value,json=newValue,proto3" json:"new_value,omitempty"`
	UpdateOp Field_UpdateOp `protobuf:"varint,5,opt,name=update_op,json=updateOp,proto3,enum=sf.substreams.sink.database.v1.Field_UpdateOp" json:"update_op,omitempty"`
}

func (x *Field) Reset()                        {}
func (x *Field) String() string                { return x.Name }
func (x *Field) ProtoMessage()                 {}
func (x *Field) ProtoReflect() protoreflect.Message { return nil }
