// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        (unknown)
// source: sf/substreams/v1/modules.proto

package substreamsv1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Module_KindStore_UpdatePolicy int32

const (
	Module_KindStore_UPDATE_POLICY_UNSET Module_KindStore_UpdatePolicy = 0
	// Provides a store where you can `set()` keys, and the latest key wins
	Module_KindStore_UPDATE_POLICY_SET Module_KindStore_UpdatePolicy = 1
	// Provides a store where you can `set_if_not_exists()` keys, and the first key wins
	Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS Module_KindStore_UpdatePolicy = 2
	// Provides a store where you can `add_*()` keys, where two stores merge by summing its values.
	Module_KindStore_UPDATE_POLICY_ADD Module_KindStore_UpdatePolicy = 3
	// Provides a store where you can `min_*()` keys, where two stores merge by leaving the minimum value.
	Module_KindStore_UPDATE_POLICY_MIN Module_KindStore_UpdatePolicy = 4
	// Provides a store where you can `max_*()` keys, where two stores merge by leaving the maximum value.
	Module_KindStore_UPDATE_POLICY_MAX Module_KindStore_UpdatePolicy = 5
	// Provides a store where you can `append()` keys, where two stores merge by concatenating the bytes in order.
	Module_KindStore_UPDATE_POLICY_APPEND Module_KindStore_UpdatePolicy = 6
)

// Enum value maps for Module_KindStore_UpdatePolicy.
var (
	Module_KindStore_UpdatePolicy_name = map[int32]string{
		0: "UPDATE_POLICY_UNSET",
		1: "UPDATE_POLICY_SET",
		2: "UPDATE_POLICY_SET_IF_NOT_EXISTS",
		3: "UPDATE_POLICY_ADD",
		4: "UPDATE_POLICY_MIN",
		5: "UPDATE_POLICY_MAX",
		6: "UPDATE_POLICY_APPEND",
	}
	Module_KindStore_UpdatePolicy_value = map[string]int32{
		"UPDATE_POLICY_UNSET":             0,
		"UPDATE_POLICY_SET":               1,
		"UPDATE_POLICY_SET_IF_NOT_EXISTS": 2,
		"UPDATE_POLICY_ADD":               3,
		"UPDATE_POLICY_MIN":               4,
		"UPDATE_POLICY_MAX":               5,
		"UPDATE_POLICY_APPEND":            6,
	}
)

func (x Module_KindStore_UpdatePolicy) Enum() *Module_KindStore_UpdatePolicy {
	p := new(Module_KindStore_UpdatePolicy)
	*p = x
	return p
}

func (x Module_KindStore_UpdatePolicy) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Module_KindStore_UpdatePolicy) Descriptor() protoreflect.EnumDescriptor {
	return file_sf_substreams_v1_modules_proto_enumTypes[0].Descriptor()
}

func (Module_KindStore_UpdatePolicy) Type() protoreflect.EnumType {
	return &file_sf_substreams_v1_modules_proto_enumTypes[0]
}

func (x Module_KindStore_UpdatePolicy) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Module_KindStore_UpdatePolicy.Descriptor instead.
func (Module_KindStore_UpdatePolicy) EnumDescriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 1, 0}
}

type Module_Input_Store_Mode int32

const (
	Module_Input_Store_UNSET  Module_Input_Store_Mode = 0
	Module_Input_Store_GET    Module_Input_Store_Mode = 1
	Module_Input_Store_DELTAS Module_Input_Store_Mode = 2
)

// Enum value maps for Module_Input_Store_Mode.
var (
	Module_Input_Store_Mode_name = map[int32]string{
		0: "UNSET",
		1: "GET",
		2: "DELTAS",
	}
	Module_Input_Store_Mode_value = map[string]int32{
		"UNSET":  0,
		"GET":    1,
		"DELTAS": 2,
	}
)

func (x Module_Input_Store_Mode) Enum() *Module_Input_Store_Mode {
	p := new(Module_Input_Store_Mode)
	*p = x
	return p
}

func (x Module_Input_Store_Mode) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Module_Input_Store_Mode) Descriptor() protoreflect.EnumDescriptor {
	return file_sf_substreams_v1_modules_proto_enumTypes[1].Descriptor()
}

func (Module_Input_Store_Mode) Type() protoreflect.EnumType {
	return &file_sf_substreams_v1_modules_proto_enumTypes[1]
}

func (x Module_Input_Store_Mode) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Module_Input_Store_Mode.Descriptor instead.
func (Module_Input_Store_Mode) EnumDescriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 2, 2, 0}
}

type Modules struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Modules  []*Module `protobuf:"bytes,1,rep,name=modules,proto3" json:"modules,omitempty"`
	Binaries []*Binary `protobuf:"bytes,2,rep,name=binaries,proto3" json:"binaries,omitempty"`
}

func (x *Modules) Reset() {
	*x = Modules{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Modules) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Modules) ProtoMessage() {}

func (x *Modules) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Modules.ProtoReflect.Descriptor instead.
func (*Modules) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{0}
}

func (x *Modules) GetModules() []*Module {
	if x != nil {
		return x.Modules
	}
	return nil
}

func (x *Modules) GetBinaries() []*Binary {
	if x != nil {
		return x.Binaries
	}
	return nil
}

// Binary represents some code compiled to its binary form.
type Binary struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type    string `protobuf:"bytes,1,opt,name=type,proto3" json:"type,omitempty"`
	Content []byte `protobuf:"bytes,2,opt,name=content,proto3" json:"content,omitempty"`
}

func (x *Binary) Reset() {
	*x = Binary{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Binary) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Binary) ProtoMessage() {}

func (x *Binary) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Binary.ProtoReflect.Descriptor instead.
func (*Binary) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{1}
}

func (x *Binary) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *Binary) GetContent() []byte {
	if x != nil {
		return x.Content
	}
	return nil
}

type Module struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Types that are assignable to Kind:
	//	*Module_KindMap_
	//	*Module_KindStore_
	Kind             isModule_Kind   `protobuf_oneof:"kind"`
	BinaryIndex      uint32          `protobuf:"varint,4,opt,name=binary_index,json=binaryIndex,proto3" json:"binary_index,omitempty"`
	BinaryEntrypoint string          `protobuf:"bytes,5,opt,name=binary_entrypoint,json=binaryEntrypoint,proto3" json:"binary_entrypoint,omitempty"`
	Inputs           []*Module_Input `protobuf:"bytes,6,rep,name=inputs,proto3" json:"inputs,omitempty"`
	Output           *Module_Output  `protobuf:"bytes,7,opt,name=output,proto3" json:"output,omitempty"`
	InitialBlock     uint64          `protobuf:"varint,8,opt,name=initial_block,json=initialBlock,proto3" json:"initial_block,omitempty"`
}

func (x *Module) Reset() {
	*x = Module{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module) ProtoMessage() {}

func (x *Module) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module.ProtoReflect.Descriptor instead.
func (*Module) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2}
}

func (x *Module) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (m *Module) GetKind() isModule_Kind {
	if m != nil {
		return m.Kind
	}
	return nil
}

func (x *Module) GetKindMap() *Module_KindMap {
	if x, ok := x.GetKind().(*Module_KindMap_); ok {
		return x.KindMap
	}
	return nil
}

func (x *Module) GetKindStore() *Module_KindStore {
	if x, ok := x.GetKind().(*Module_KindStore_); ok {
		return x.KindStore
	}
	return nil
}

func (x *Module) GetBinaryIndex() uint32 {
	if x != nil {
		return x.BinaryIndex
	}
	return 0
}

func (x *Module) GetBinaryEntrypoint() string {
	if x != nil {
		return x.BinaryEntrypoint
	}
	return ""
}

func (x *Module) GetInputs() []*Module_Input {
	if x != nil {
		return x.Inputs
	}
	return nil
}

func (x *Module) GetOutput() *Module_Output {
	if x != nil {
		return x.Output
	}
	return nil
}

func (x *Module) GetInitialBlock() uint64 {
	if x != nil {
		return x.InitialBlock
	}
	return 0
}

type isModule_Kind interface {
	isModule_Kind()
}

type Module_KindMap_ struct {
	KindMap *Module_KindMap `protobuf:"bytes,2,opt,name=kind_map,json=kindMap,proto3,oneof"`
}

type Module_KindStore_ struct {
	KindStore *Module_KindStore `protobuf:"bytes,3,opt,name=kind_store,json=kindStore,proto3,oneof"`
}

func (*Module_KindMap_) isModule_Kind() {}

func (*Module_KindStore_) isModule_Kind() {}

type Module_KindMap struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	OutputType string `protobuf:"bytes,1,opt,name=output_type,json=outputType,proto3" json:"output_type,omitempty"`
}

func (x *Module_KindMap) Reset() {
	*x = Module_KindMap{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module_KindMap) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module_KindMap) ProtoMessage() {}

func (x *Module_KindMap) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module_KindMap.ProtoReflect.Descriptor instead.
func (*Module_KindMap) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 0}
}

func (x *Module_KindMap) GetOutputType() string {
	if x != nil {
		return x.OutputType
	}
	return ""
}

type Module_KindStore struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The `update_policy` determines the functions available to mutate the store
	// (like `set()`, `set_if_not_exists()` or `sum()`, etc..) in
	// order to ensure that parallel operations are possible and deterministic
	//
	// Say a store cumulates keys from block 0 to 1M, and a second store
	// cumulates keys from block 1M to 2M. When we want to use this
	// store as a dependency for a downstream module, we will merge the
	// two stores according to this policy.
	UpdatePolicy Module_KindStore_UpdatePolicy `protobuf:"varint,1,opt,name=update_policy,json=updatePolicy,proto3,enum=sf.substreams.v1.Module_KindStore_UpdatePolicy" json:"update_policy,omitempty"`
	ValueType    string                        `protobuf:"bytes,2,opt,name=value_type,json=valueType,proto3" json:"value_type,omitempty"`
}

func (x *Module_KindStore) Reset() {
	*x = Module_KindStore{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module_KindStore) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module_KindStore) ProtoMessage() {}

func (x *Module_KindStore) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module_KindStore.ProtoReflect.Descriptor instead.
func (*Module_KindStore) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 1}
}

func (x *Module_KindStore) GetUpdatePolicy() Module_KindStore_UpdatePolicy {
	if x != nil {
		return x.UpdatePolicy
	}
	return Module_KindStore_UPDATE_POLICY_UNSET
}

func (x *Module_KindStore) GetValueType() string {
	if x != nil {
		return x.ValueType
	}
	return ""
}

type Module_Input struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Input:
	//	*Module_Input_Source_
	//	*Module_Input_Map_
	//	*Module_Input_Store_
	Input isModule_Input_Input `protobuf_oneof:"input"`
}

func (x *Module_Input) Reset() {
	*x = Module_Input{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module_Input) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module_Input) ProtoMessage() {}

func (x *Module_Input) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module_Input.ProtoReflect.Descriptor instead.
func (*Module_Input) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 2}
}

func (m *Module_Input) GetInput() isModule_Input_Input {
	if m != nil {
		return m.Input
	}
	return nil
}

func (x *Module_Input) GetSource() *Module_Input_Source {
	if x, ok := x.GetInput().(*Module_Input_Source_); ok {
		return x.Source
	}
	return nil
}

func (x *Module_Input) GetMap() *Module_Input_Map {
	if x, ok := x.GetInput().(*Module_Input_Map_); ok {
		return x.Map
	}
	return nil
}

func (x *Module_Input) GetStore() *Module_Input_Store {
	if x, ok := x.GetInput().(*Module_Input_Store_); ok {
		return x.Store
	}
	return nil
}

type isModule_Input_Input interface {
	isModule_Input_Input()
}

type Module_Input_Source_ struct {
	Source *Module_Input_Source `protobuf:"bytes,1,opt,name=source,proto3,oneof"`
}

type Module_Input_Map_ struct {
	Map *Module_Input_Map `protobuf:"bytes,2,opt,name=map,proto3,oneof"`
}

type Module_Input_Store_ struct {
	Store *Module_Input_Store `protobuf:"bytes,3,opt,name=store,proto3,oneof"`
}

func (*Module_Input_Source_) isModule_Input_Input() {}

func (*Module_Input_Map_) isModule_Input_Input() {}

func (*Module_Input_Store_) isModule_Input_Input() {}

type Module_Output struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type string `protobuf:"bytes,1,opt,name=type,proto3" json:"type,omitempty"`
}

func (x *Module_Output) Reset() {
	*x = Module_Output{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module_Output) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module_Output) ProtoMessage() {}

func (x *Module_Output) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module_Output.ProtoReflect.Descriptor instead.
func (*Module_Output) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 3}
}

func (x *Module_Output) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

type Module_Input_Source struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type string `protobuf:"bytes,1,opt,name=type,proto3" json:"type,omitempty"` // ex: "sf.ethereum.type.v1.Block"
}

func (x *Module_Input_Source) Reset() {
	*x = Module_Input_Source{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module_Input_Source) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module_Input_Source) ProtoMessage() {}

func (x *Module_Input_Source) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module_Input_Source.ProtoReflect.Descriptor instead.
func (*Module_Input_Source) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 2, 0}
}

func (x *Module_Input_Source) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

type Module_Input_Map struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ModuleName string `protobuf:"bytes,1,opt,name=module_name,json=moduleName,proto3" json:"module_name,omitempty"` // ex: "block_to_pairs"
}

func (x *Module_Input_Map) Reset() {
	*x = Module_Input_Map{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module_Input_Map) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module_Input_Map) ProtoMessage() {}

func (x *Module_Input_Map) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module_Input_Map.ProtoReflect.Descriptor instead.
func (*Module_Input_Map) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 2, 1}
}

func (x *Module_Input_Map) GetModuleName() string {
	if x != nil {
		return x.ModuleName
	}
	return ""
}

type Module_Input_Store struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ModuleName string                  `protobuf:"bytes,1,opt,name=module_name,json=moduleName,proto3" json:"module_name,omitempty"`
	Mode       Module_Input_Store_Mode `protobuf:"varint,2,opt,name=mode,proto3,enum=sf.substreams.v1.Module_Input_Store_Mode" json:"mode,omitempty"`
}

func (x *Module_Input_Store) Reset() {
	*x = Module_Input_Store{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sf_substreams_v1_modules_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Module_Input_Store) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module_Input_Store) ProtoMessage() {}

func (x *Module_Input_Store) ProtoReflect() protoreflect.Message {
	mi := &file_sf_substreams_v1_modules_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module_Input_Store.ProtoReflect.Descriptor instead.
func (*Module_Input_Store) Descriptor() ([]byte, []int) {
	return file_sf_substreams_v1_modules_proto_rawDescGZIP(), []int{2, 2, 2}
}

func (x *Module_Input_Store) GetModuleName() string {
	if x != nil {
		return x.ModuleName
	}
	return ""
}

func (x *Module_Input_Store) GetMode() Module_Input_Store_Mode {
	if x != nil {
		return x.Mode
	}
	return Module_Input_Store_UNSET
}

var File_sf_substreams_v1_modules_proto protoreflect.FileDescriptor

var file_sf_substreams_v1_modules_proto_rawDesc = []byte{
	0x0a, 0x1e, 0x73, 0x66, 0x2f, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2f,
	0x76, 0x31, 0x2f, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x10, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e,
	0x76, 0x31, 0x22, 0x73, 0x0a, 0x07, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x73, 0x12, 0x32, 0x0a,
	0x07, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x18,
	0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76,
	0x31, 0x2e, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x52, 0x07, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65,
	0x73, 0x12, 0x34, 0x0a, 0x08, 0x62, 0x69, 0x6e, 0x61, 0x72, 0x69, 0x65, 0x73, 0x18, 0x02, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65,
	0x61, 0x6d, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x42, 0x69, 0x6e, 0x61, 0x72, 0x79, 0x52, 0x08, 0x62,
	0x69, 0x6e, 0x61, 0x72, 0x69, 0x65, 0x73, 0x22, 0x36, 0x0a, 0x06, 0x42, 0x69, 0x6e, 0x61, 0x72,
	0x79, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x22,
	0xc2, 0x09, 0x0a, 0x06, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x3d,
	0x0a, 0x08, 0x6b, 0x69, 0x6e, 0x64, 0x5f, 0x6d, 0x61, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x20, 0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73,
	0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x2e, 0x4b, 0x69, 0x6e, 0x64, 0x4d,
	0x61, 0x70, 0x48, 0x00, 0x52, 0x07, 0x6b, 0x69, 0x6e, 0x64, 0x4d, 0x61, 0x70, 0x12, 0x43, 0x0a,
	0x0a, 0x6b, 0x69, 0x6e, 0x64, 0x5f, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x22, 0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d,
	0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x2e, 0x4b, 0x69, 0x6e, 0x64,
	0x53, 0x74, 0x6f, 0x72, 0x65, 0x48, 0x00, 0x52, 0x09, 0x6b, 0x69, 0x6e, 0x64, 0x53, 0x74, 0x6f,
	0x72, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x62, 0x69, 0x6e, 0x61, 0x72, 0x79, 0x5f, 0x69, 0x6e, 0x64,
	0x65, 0x78, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0b, 0x62, 0x69, 0x6e, 0x61, 0x72, 0x79,
	0x49, 0x6e, 0x64, 0x65, 0x78, 0x12, 0x2b, 0x0a, 0x11, 0x62, 0x69, 0x6e, 0x61, 0x72, 0x79, 0x5f,
	0x65, 0x6e, 0x74, 0x72, 0x79, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x10, 0x62, 0x69, 0x6e, 0x61, 0x72, 0x79, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x70, 0x6f, 0x69,
	0x6e, 0x74, 0x12, 0x36, 0x0a, 0x06, 0x69, 0x6e, 0x70, 0x75, 0x74, 0x73, 0x18, 0x06, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x2e, 0x49, 0x6e, 0x70,
	0x75, 0x74, 0x52, 0x06, 0x69, 0x6e, 0x70, 0x75, 0x74, 0x73, 0x12, 0x37, 0x0a, 0x06, 0x6f, 0x75,
	0x74, 0x70, 0x75, 0x74, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1f, 0x2e, 0x73, 0x66, 0x2e,
	0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x6f,
	0x64, 0x75, 0x6c, 0x65, 0x2e, 0x4f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x52, 0x06, 0x6f, 0x75, 0x74,
	0x70, 0x75, 0x74, 0x12, 0x23, 0x0a, 0x0d, 0x69, 0x6e, 0x69, 0x74, 0x69, 0x61, 0x6c, 0x5f, 0x62,
	0x6c, 0x6f, 0x63, 0x6b, 0x18, 0x08, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0c, 0x69, 0x6e, 0x69, 0x74,
	0x69, 0x61, 0x6c, 0x42, 0x6c, 0x6f, 0x63, 0x6b, 0x1a, 0x2a, 0x0a, 0x07, 0x4b, 0x69, 0x6e, 0x64,
	0x4d, 0x61, 0x70, 0x12, 0x1f, 0x0a, 0x0b, 0x6f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x5f, 0x74, 0x79,
	0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x6f, 0x75, 0x74, 0x70, 0x75, 0x74,
	0x54, 0x79, 0x70, 0x65, 0x1a, 0xc5, 0x02, 0x0a, 0x09, 0x4b, 0x69, 0x6e, 0x64, 0x53, 0x74, 0x6f,
	0x72, 0x65, 0x12, 0x54, 0x0a, 0x0d, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x5f, 0x70, 0x6f, 0x6c,
	0x69, 0x63, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x2f, 0x2e, 0x73, 0x66, 0x2e, 0x73,
	0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x6f, 0x64,
	0x75, 0x6c, 0x65, 0x2e, 0x4b, 0x69, 0x6e, 0x64, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x55, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x50, 0x6f, 0x6c, 0x69, 0x63, 0x79, 0x52, 0x0c, 0x75, 0x70, 0x64, 0x61,
	0x74, 0x65, 0x50, 0x6f, 0x6c, 0x69, 0x63, 0x79, 0x12, 0x1d, 0x0a, 0x0a, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x76, 0x61,
	0x6c, 0x75, 0x65, 0x54, 0x79, 0x70, 0x65, 0x22, 0xc2, 0x01, 0x0a, 0x0c, 0x55, 0x70, 0x64, 0x61,
	0x74, 0x65, 0x50, 0x6f, 0x6c, 0x69, 0x63, 0x79, 0x12, 0x17, 0x0a, 0x13, 0x55, 0x50, 0x44, 0x41,
	0x54, 0x45, 0x5f, 0x50, 0x4f, 0x4c, 0x49, 0x43, 0x59, 0x5f, 0x55, 0x4e, 0x53, 0x45, 0x54, 0x10,
	0x00, 0x12, 0x15, 0x0a, 0x11, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45, 0x5f, 0x50, 0x4f, 0x4c, 0x49,
	0x43, 0x59, 0x5f, 0x53, 0x45, 0x54, 0x10, 0x01, 0x12, 0x23, 0x0a, 0x1f, 0x55, 0x50, 0x44, 0x41,
	0x54, 0x45, 0x5f, 0x50, 0x4f, 0x4c, 0x49, 0x43, 0x59, 0x5f, 0x53, 0x45, 0x54, 0x5f, 0x49, 0x46,
	0x5f, 0x4e, 0x4f, 0x54, 0x5f, 0x45, 0x58, 0x49, 0x53, 0x54, 0x53, 0x10, 0x02, 0x12, 0x15, 0x0a,
	0x11, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45, 0x5f, 0x50, 0x4f, 0x4c, 0x49, 0x43, 0x59, 0x5f, 0x41,
	0x44, 0x44, 0x10, 0x03, 0x12, 0x15, 0x0a, 0x11, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45, 0x5f, 0x50,
	0x4f, 0x4c, 0x49, 0x43, 0x59, 0x5f, 0x4d, 0x49, 0x4e, 0x10, 0x04, 0x12, 0x15, 0x0a, 0x11, 0x55,
	0x50, 0x44, 0x41, 0x54, 0x45, 0x5f, 0x50, 0x4f, 0x4c, 0x49, 0x43, 0x59, 0x5f, 0x4d, 0x41, 0x58,
	0x10, 0x05, 0x12, 0x18, 0x0a, 0x14, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45, 0x5f, 0x50, 0x4f, 0x4c,
	0x49, 0x43, 0x59, 0x5f, 0x41, 0x50, 0x50, 0x45, 0x4e, 0x44, 0x10, 0x06, 0x1a, 0x9f, 0x03, 0x0a,
	0x05, 0x49, 0x6e, 0x70, 0x75, 0x74, 0x12, 0x3f, 0x0a, 0x06, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73,
	0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65,
	0x2e, 0x49, 0x6e, 0x70, 0x75, 0x74, 0x2e, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x48, 0x00, 0x52,
	0x06, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x36, 0x0a, 0x03, 0x6d, 0x61, 0x70, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x22, 0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x2e, 0x49,
	0x6e, 0x70, 0x75, 0x74, 0x2e, 0x4d, 0x61, 0x70, 0x48, 0x00, 0x52, 0x03, 0x6d, 0x61, 0x70, 0x12,
	0x3c, 0x0a, 0x05, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x24,
	0x2e, 0x73, 0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76,
	0x31, 0x2e, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x2e, 0x49, 0x6e, 0x70, 0x75, 0x74, 0x2e, 0x53,
	0x74, 0x6f, 0x72, 0x65, 0x48, 0x00, 0x52, 0x05, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x1a, 0x1c, 0x0a,
	0x06, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x1a, 0x26, 0x0a, 0x03, 0x4d,
	0x61, 0x70, 0x12, 0x1f, 0x0a, 0x0b, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d,
	0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x4e,
	0x61, 0x6d, 0x65, 0x1a, 0x8f, 0x01, 0x0a, 0x05, 0x53, 0x74, 0x6f, 0x72, 0x65, 0x12, 0x1f, 0x0a,
	0x0b, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0a, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x3d,
	0x0a, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x29, 0x2e, 0x73,
	0x66, 0x2e, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76, 0x31, 0x2e,
	0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x2e, 0x49, 0x6e, 0x70, 0x75, 0x74, 0x2e, 0x53, 0x74, 0x6f,
	0x72, 0x65, 0x2e, 0x4d, 0x6f, 0x64, 0x65, 0x52, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x22, 0x26, 0x0a,
	0x04, 0x4d, 0x6f, 0x64, 0x65, 0x12, 0x09, 0x0a, 0x05, 0x55, 0x4e, 0x53, 0x45, 0x54, 0x10, 0x00,
	0x12, 0x07, 0x0a, 0x03, 0x47, 0x45, 0x54, 0x10, 0x01, 0x12, 0x0a, 0x0a, 0x06, 0x44, 0x45, 0x4c,
	0x54, 0x41, 0x53, 0x10, 0x02, 0x42, 0x07, 0x0a, 0x05, 0x69, 0x6e, 0x70, 0x75, 0x74, 0x1a, 0x1c,
	0x0a, 0x06, 0x4f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x42, 0x06, 0x0a, 0x04,
	0x6b, 0x69, 0x6e, 0x64, 0x42, 0xcc, 0x01, 0x0a, 0x14, 0x63, 0x6f, 0x6d, 0x2e, 0x73, 0x66, 0x2e,
	0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x76, 0x31, 0x42, 0x0c, 0x4d,
	0x6f, 0x64, 0x75, 0x6c, 0x65, 0x73, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x44, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d,
	0x69, 0x6e, 0x67, 0x66, 0x61, 0x73, 0x74, 0x2f, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x73, 0x2f, 0x70, 0x62, 0x2f, 0x73, 0x66, 0x2f, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65,
	0x61, 0x6d, 0x73, 0x2f, 0x76, 0x31, 0x3b, 0x73, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d,
	0x73, 0x76, 0x31, 0xa2, 0x02, 0x03, 0x53, 0x53, 0x58, 0xaa, 0x02, 0x10, 0x53, 0x66, 0x2e, 0x53,
	0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x10, 0x53,
	0x66, 0x5c, 0x53, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x5c, 0x56, 0x31, 0xe2,
	0x02, 0x1c, 0x53, 0x66, 0x5c, 0x53, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x5c,
	0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02,
	0x12, 0x53, 0x66, 0x3a, 0x3a, 0x53, 0x75, 0x62, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x73, 0x3a,
	0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_sf_substreams_v1_modules_proto_rawDescOnce sync.Once
	file_sf_substreams_v1_modules_proto_rawDescData = file_sf_substreams_v1_modules_proto_rawDesc
)

func file_sf_substreams_v1_modules_proto_rawDescGZIP() []byte {
	file_sf_substreams_v1_modules_proto_rawDescOnce.Do(func() {
		file_sf_substreams_v1_modules_proto_rawDescData = protoimpl.X.CompressGZIP(file_sf_substreams_v1_modules_proto_rawDescData)
	})
	return file_sf_substreams_v1_modules_proto_rawDescData
}

var file_sf_substreams_v1_modules_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_sf_substreams_v1_modules_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_sf_substreams_v1_modules_proto_goTypes = []interface{}{
	(Module_KindStore_UpdatePolicy)(0), // 0: sf.substreams.v1.Module.KindStore.UpdatePolicy
	(Module_Input_Store_Mode)(0),       // 1: sf.substreams.v1.Module.Input.Store.Mode
	(*Modules)(nil),                    // 2: sf.substreams.v1.Modules
	(*Binary)(nil),                     // 3: sf.substreams.v1.Binary
	(*Module)(nil),                     // 4: sf.substreams.v1.Module
	(*Module_KindMap)(nil),             // 5: sf.substreams.v1.Module.KindMap
	(*Module_KindStore)(nil),           // 6: sf.substreams.v1.Module.KindStore
	(*Module_Input)(nil),               // 7: sf.substreams.v1.Module.Input
	(*Module_Output)(nil),              // 8: sf.substreams.v1.Module.Output
	(*Module_Input_Source)(nil),        // 9: sf.substreams.v1.Module.Input.Source
	(*Module_Input_Map)(nil),           // 10: sf.substreams.v1.Module.Input.Map
	(*Module_Input_Store)(nil),         // 11: sf.substreams.v1.Module.Input.Store
}
var file_sf_substreams_v1_modules_proto_depIdxs = []int32{
	4,  // 0: sf.substreams.v1.Modules.modules:type_name -> sf.substreams.v1.Module
	3,  // 1: sf.substreams.v1.Modules.binaries:type_name -> sf.substreams.v1.Binary
	5,  // 2: sf.substreams.v1.Module.kind_map:type_name -> sf.substreams.v1.Module.KindMap
	6,  // 3: sf.substreams.v1.Module.kind_store:type_name -> sf.substreams.v1.Module.KindStore
	7,  // 4: sf.substreams.v1.Module.inputs:type_name -> sf.substreams.v1.Module.Input
	8,  // 5: sf.substreams.v1.Module.output:type_name -> sf.substreams.v1.Module.Output
	0,  // 6: sf.substreams.v1.Module.KindStore.update_policy:type_name -> sf.substreams.v1.Module.KindStore.UpdatePolicy
	9,  // 7: sf.substreams.v1.Module.Input.source:type_name -> sf.substreams.v1.Module.Input.Source
	10, // 8: sf.substreams.v1.Module.Input.map:type_name -> sf.substreams.v1.Module.Input.Map
	11, // 9: sf.substreams.v1.Module.Input.store:type_name -> sf.substreams.v1.Module.Input.Store
	1,  // 10: sf.substreams.v1.Module.Input.Store.mode:type_name -> sf.substreams.v1.Module.Input.Store.Mode
	11, // [11:11] is the sub-list for method output_type
	11, // [11:11] is the sub-list for method input_type
	11, // [11:11] is the sub-list for extension type_name
	11, // [11:11] is the sub-list for extension extendee
	0,  // [0:11] is the sub-list for field type_name
}

func init() { file_sf_substreams_v1_modules_proto_init() }
func file_sf_substreams_v1_modules_proto_init() {
	if File_sf_substreams_v1_modules_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_sf_substreams_v1_modules_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Modules); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Binary); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module_KindMap); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module_KindStore); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module_Input); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module_Output); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module_Input_Source); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module_Input_Map); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sf_substreams_v1_modules_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Module_Input_Store); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_sf_substreams_v1_modules_proto_msgTypes[2].OneofWrappers = []interface{}{
		(*Module_KindMap_)(nil),
		(*Module_KindStore_)(nil),
	}
	file_sf_substreams_v1_modules_proto_msgTypes[5].OneofWrappers = []interface{}{
		(*Module_Input_Source_)(nil),
		(*Module_Input_Map_)(nil),
		(*Module_Input_Store_)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_sf_substreams_v1_modules_proto_rawDesc,
			NumEnums:      2,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_sf_substreams_v1_modules_proto_goTypes,
		DependencyIndexes: file_sf_substreams_v1_modules_proto_depIdxs,
		EnumInfos:         file_sf_substreams_v1_modules_proto_enumTypes,
		MessageInfos:      file_sf_substreams_v1_modules_proto_msgTypes,
	}.Build()
	File_sf_substreams_v1_modules_proto = out.File
	file_sf_substreams_v1_modules_proto_rawDesc = nil
	file_sf_substreams_v1_modules_proto_goTypes = nil
	file_sf_substreams_v1_modules_proto_depIdxs = nil
}
