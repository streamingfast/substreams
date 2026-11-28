package rows

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/streamingfast/protox"
	schemapb "github.com/streamingfast/substreams/pb/sf/substreams/sink/v1"
)

const timestampFullName protoreflect.FullName = "google.protobuf.Timestamp"

// planKind says what the walk does with a field.
type planKind uint8

const (
	planColumn planKind = iota + 1
	planChild
	planChildList
)

type fieldPlan struct {
	field protoreflect.FieldDescriptor
	kind  planKind
	col   ColumnID // planColumn
	// presence is true when the column value can be absent and Has decides it.
	presence bool
	child    *msgPlan // planChild, planChildList
	check    checkKind
}

// msgPlan is the pre-computed walk of one message type, so the walk never reads descriptors' options.
type msgPlan struct {
	md     protoreflect.MessageDescriptor
	table  *Table // nil for a pass-through message, set once the schema is final
	tid    TableID
	hasTbl bool
	fields []fieldPlan
	// children are the tables directly reachable through pass-through children, filled after resolution.
	children []TableID
}

// Option configures resolution and producers.
type Option func(*options)

type options struct {
	requireAnnotations bool
	check              bool
	moduleName         string
	schema             *Schema
}

func newOptions(opts []Option) options {
	o := options{requireAnnotations: true}
	for _, apply := range opts {
		apply(&o)
	}
	return o
}

// WithRequireAnnotations sets whether only messages carrying a table annotation become tables (default true).
// When false, every message that has columns becomes a table named after the message.
func WithRequireAnnotations(required bool) Option {
	return func(o *options) { o.requireAnnotations = required }
}

// WithCheck makes the walk validate bigint and decimal columns, see [CheckBigInt] and [CheckDecimal]. It is off by default.
func WithCheck() Option {
	return func(o *options) { o.check = true }
}

// WithModuleName names the output module, it is the table name of the [BlockRowProducer].
func WithModuleName(name string) Option {
	return func(o *options) { o.moduleName = name }
}

// Resolve builds the schema of an output message. It reads the `(table)` and `(field)` annotations once, so the
// walk never does.
//
// Recursive message types are rejected, as are inconsistent annotations (see the error messages), so a bad schema
// fails at startup rather than on the first row.
func Resolve(root protoreflect.MessageDescriptor, opts ...Option) (*Schema, error) {
	o := newOptions(opts)
	if root == nil {
		return nil, errors.New("nil root message descriptor")
	}

	r := &resolver{
		requireAnnotations: o.requireAnnotations,
		schema:             &Schema{Root: root},
		plans:              map[protoreflect.FullName]*msgPlan{},
		visiting:           map[protoreflect.FullName]bool{},
		tableMessage:       map[string]protoreflect.FullName{},
	}

	rootPlan, err := r.plan(root)
	if err != nil {
		return nil, fmt.Errorf("resolving schema of %q: %w", root.FullName(), err)
	}

	s := r.schema
	if len(s.Tables) == 0 {
		return nil, fmt.Errorf("no table found in message %q: annotate messages with `option (schema.table) = { name: \"...\" }`", root.FullName())
	}

	s.index()
	if err := r.validateRelations(); err != nil {
		return nil, err
	}

	// The tables slice is final, so plans can point into it.
	for _, p := range r.plans {
		if p.hasTbl {
			p.table = &s.Tables[p.tid]
		}
	}
	for i := range s.Tables {
		s.maxColumns = max(s.maxColumns, len(s.Tables[i].Columns))
	}
	for _, p := range r.plans {
		p.children = r.reachableTables(p)
	}
	for i := range s.Tables {
		s.Tables[i].Children = r.plans[s.Tables[i].Message.FullName()].children
	}

	s.rootPlan = rootPlan
	return s, nil
}

type resolver struct {
	requireAnnotations bool
	schema             *Schema
	plans              map[protoreflect.FullName]*msgPlan
	visiting           map[protoreflect.FullName]bool
	tableMessage       map[string]protoreflect.FullName
}

// tableOption and fieldOption read the schema annotations, nil when the descriptor has none.
func tableOption(md protoreflect.MessageDescriptor) *schemapb.Table {
	table, _ := protox.GetMessageExtensionValue[*schemapb.Table](md, schemapb.E_Table, nil)
	return table
}

func fieldOption(fd protoreflect.FieldDescriptor) *schemapb.Column {
	column, _ := protox.GetFieldExtensionValue[*schemapb.Column](fd, schemapb.E_Field, nil)
	return column
}

func isTimestamp(fd protoreflect.FieldDescriptor) bool {
	return protox.IsWellKnownTimestampField(fd)
}

// isIgnoredWellKnown tells a google.protobuf message other than Timestamp, which is neither a table nor a column unless inline.
func isIgnoredWellKnown(fd protoreflect.FieldDescriptor) bool {
	return protox.IsWellKnownGoogleField(fd) && !isTimestamp(fd)
}

// isChildField tells whether a field holds a message that is resolved as a table or a pass-through message: a message that
// is not a map, a Timestamp, another well-known message or an inline column.
func isChildField(fd protoreflect.FieldDescriptor) bool {
	return fd.Kind() == protoreflect.MessageKind && !fd.IsMap() && !isTimestamp(fd) && !isIgnoredWellKnown(fd) && !isInline(fd)
}

func isInline(fd protoreflect.FieldDescriptor) bool {
	if fd.Kind() != protoreflect.MessageKind {
		return false
	}

	info := fieldOption(fd)
	return info != nil && info.Inline
}

// plan resolves a message and, first, everything under it (post-order, as db_proto registers its tables).
func (r *resolver) plan(md protoreflect.MessageDescriptor) (*msgPlan, error) {
	full := md.FullName()
	if p, found := r.plans[full]; found {
		return p, nil
	}
	if r.visiting[full] {
		return nil, fmt.Errorf("recursive message type %q is not supported", full)
	}
	r.visiting[full] = true
	defer delete(r.visiting, full)

	p := &msgPlan{md: md}

	// Children first, so table ids follow post-order discovery.
	fields := md.Fields()
	children := make([]*msgPlan, fields.Len())
	for i := range fields.Len() {
		fd := fields.Get(i)
		if !isChildField(fd) {
			continue
		}

		child, err := r.plan(fd.Message())
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", fd.Name(), err)
		}
		children[i] = child
	}

	info := tableOption(md)
	isTable := info != nil || !r.requireAnnotations
	if md.FullName() == timestampFullName {
		isTable = false
	}

	var table Table
	columns := make([]Column, 0, fields.Len())
	for i := range fields.Len() {
		fd := fields.Get(i)

		switch {
		case fd.IsMap(), isIgnoredWellKnown(fd) && !isInline(fd):
			// Maps and well-known messages other than Timestamp (Duration, Any, wrappers, Struct...) have no column and
			// no table, as in db_proto: annotate the field `inline` to keep it as a struct column.
			continue

		case fd.Kind() == protoreflect.MessageKind && children[i] != nil:
			kind := planChild
			if fd.IsList() {
				kind = planChildList
			}
			p.fields = append(p.fields, fieldPlan{field: fd, kind: kind, child: children[i], presence: !fd.IsList()})
			continue
		}

		column, check, err := newColumn(fd, len(columns))
		if err != nil {
			return nil, fmt.Errorf("message %q: %w", md.FullName(), err)
		}

		p.fields = append(p.fields, fieldPlan{field: fd, kind: planColumn, col: column.ID, presence: column.Optional, check: check})
		columns = append(columns, column)
	}

	if isTable && len(columns) > 0 {
		table = Table{Message: md, Columns: columns, Name: string(md.Name())}
		if info != nil && info.Name != "" {
			table.Name = info.Name
		}
		if info != nil && info.ChildOf != nil {
			childOf, err := parseChildOf(info.GetChildOf())
			if err != nil {
				return nil, fmt.Errorf("table %q: %w", table.Name, err)
			}
			table.HasChildOf, table.ChildOf = true, childOf
		}

		if err := table.resolveKey(info); err != nil {
			return nil, err
		}
		if err := table.checkColumns(); err != nil {
			return nil, err
		}

		if known, found := r.tableMessage[table.Name]; found && known != full {
			return nil, fmt.Errorf("table name %q is used by messages %q and %q", table.Name, known, full)
		}
		r.tableMessage[table.Name] = full

		table.ID = TableID(len(r.schema.Tables))
		r.schema.Tables = append(r.schema.Tables, table)
		p.hasTbl, p.tid = true, table.ID
	}

	r.plans[full] = p
	return p, nil
}

// newColumn builds the column of a field, or fails on an inconsistent annotation.
func newColumn(fd protoreflect.FieldDescriptor, id int) (Column, checkKind, error) {
	info := fieldOption(fd)

	column := Column{
		ID:       ColumnID(id),
		Name:     string(fd.Name()),
		Field:    fd,
		Optional: fd.HasOptionalKeyword() || fd.ContainingOneof() != nil || fd.Kind() == protoreflect.MessageKind && !fd.IsList(),
	}

	typ, check, err := logicalType(fd, info)
	if err != nil {
		return Column{}, 0, fmt.Errorf("column %q: %w", fd.Name(), err)
	}
	column.Type = typ

	if info != nil {
		if info.Name != nil {
			column.Name = info.GetName()
		}
		column.PK, column.Unique = info.PrimaryKey, info.Unique
		if info.ForeignKey != nil {
			fk, err := parseForeignKey(info.GetForeignKey())
			if err != nil {
				return Column{}, 0, fmt.Errorf("column %q: %w", fd.Name(), err)
			}
			column.HasFK, column.FK = true, fk
		}
	}

	// Only one level of inline nesting: an inline message holds its fields as one value, not as tables.
	if info != nil && info.Inline {
		inner := fd.Message().Fields()
		for i := range inner.Len() {
			if isInline(inner.Get(i)) {
				return Column{}, 0, fmt.Errorf("column %q: inline nesting level 2 is not supported, only 1 level of inline nesting is allowed", fd.Name())
			}
		}
	}

	return column, check, nil
}

func scalarKind(k protoreflect.Kind) (Kind, bool) {
	switch k {
	case protoreflect.StringKind:
		return KindString, true
	case protoreflect.BoolKind:
		return KindBool, true
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return KindInt32, true
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return KindInt64, true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return KindUint32, true
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return KindUint64, true
	case protoreflect.FloatKind:
		return KindFloat32, true
	case protoreflect.DoubleKind:
		return KindFloat64, true
	case protoreflect.BytesKind:
		return KindBytes, true
	case protoreflect.EnumKind:
		return KindEnum, true
	}
	return 0, false
}

// logicalType maps a field to its type. Messages here are Timestamp or inline, children were handled by the caller.
func logicalType(fd protoreflect.FieldDescriptor, info *schemapb.Column) (LogicalType, checkKind, error) {
	var elem LogicalType
	check := checkNone

	switch {
	case fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == timestampFullName:
		elem = LogicalType{Kind: KindTimestamp}
	case fd.Kind() == protoreflect.MessageKind:
		elem = LogicalType{Kind: KindStruct, Struct: fd.Message()}
	default:
		kind, ok := scalarKind(fd.Kind())
		if !ok {
			return LogicalType{}, 0, fmt.Errorf("unsupported protobuf kind %s", fd.Kind())
		}
		elem = LogicalType{Kind: kind}
		if kind == KindEnum {
			elem.Enum = fd.Enum()
		}
	}

	if conv := info.GetConvertTo(); conv != nil {
		if elem.Kind != KindString {
			return LogicalType{}, 0, fmt.Errorf("convertTo is only valid on string fields, not %s", elem.Kind)
		}

		switch {
		case conv.GetInt128() != nil:
			elem, check = LogicalType{Kind: KindBigint, Bits: 128, Signed: true}, checkBigInt
		case conv.GetUint128() != nil:
			elem, check = LogicalType{Kind: KindBigint, Bits: 128}, checkBigInt
		case conv.GetInt256() != nil:
			elem, check = LogicalType{Kind: KindBigint, Bits: 256, Signed: true}, checkBigInt
		case conv.GetUint256() != nil:
			elem, check = LogicalType{Kind: KindBigint, Bits: 256}, checkBigInt
		case conv.GetDecimal128() != nil:
			elem, check = LogicalType{Kind: KindDecimal, Bits: 128, Scale: conv.GetDecimal128().GetScale()}, checkDecimal
		case conv.GetDecimal256() != nil:
			elem, check = LogicalType{Kind: KindDecimal, Bits: 256, Scale: conv.GetDecimal256().GetScale()}, checkDecimal
		}

		if elem.Kind == KindDecimal {
			maxScale := int32(38)
			if elem.Bits == 256 {
				maxScale = 76
			}
			if elem.Scale < 0 || elem.Scale > maxScale {
				return LogicalType{}, 0, fmt.Errorf("decimal%d scale %d out of range [0, %d]", elem.Bits, elem.Scale, maxScale)
			}
		}
	}

	if !fd.IsList() {
		return elem, check, nil
	}

	list := LogicalType{Kind: KindList, Elem: elem.Kind, Bits: elem.Bits, Signed: elem.Signed, Scale: elem.Scale, Enum: elem.Enum, Struct: elem.Struct}
	return list, check, nil
}

func parseChildOf(value string) (ChildOf, error) {
	table, field, err := parseOn(value, "child_of")
	return ChildOf{ParentTable: table, ParentField: field}, err
}

func parseForeignKey(value string) (ForeignKey, error) {
	table, field, err := parseOn(value, "foreign key")
	return ForeignKey{Table: table, Field: field}, err
}

// parseOn parses the `table on field` form shared by child_of and foreign_key.
func parseOn(value, what string) (table, field string, err error) {
	left, right, found := strings.Cut(value, " on ")
	if !found || strings.Contains(right, " on ") {
		return "", "", fmt.Errorf("invalid %s %q, expecting 'table_name on field_name'", what, value)
	}
	return strings.TrimSpace(left), strings.TrimSpace(right), nil
}

// resolveKey sets the primary key. The table-level option (`primary_key_column` or `composite_primary_key`) wins when
// present; otherwise the columns flagged `primary_key` form it: one flagged column is a single key, several are a
// composite key in column order. A flagged column that the table-level option leaves out is an error.
func (t *Table) resolveKey(info *schemapb.Table) error {
	var declared []string
	switch {
	case info != nil && info.GetPrimaryKey() != nil && info.GetPrimaryKeyColumn() == "" && info.GetCompositePrimaryKey() == nil:
		return fmt.Errorf("table %q: primary_key_column is empty", t.Name)
	case info.GetPrimaryKeyColumn() != "":
		declared = []string{info.GetPrimaryKeyColumn()}
	case info.GetCompositePrimaryKey() != nil:
		declared = info.GetCompositePrimaryKey().GetColumns()
		if len(declared) < 2 {
			return fmt.Errorf("table %q: a composite primary key needs at least 2 columns, got %d", t.Name, len(declared))
		}
	}

	var keys []ColumnID
	if declared != nil {
		for _, name := range declared {
			column := t.columnByNameOrField(name)
			if column == nil {
				return fmt.Errorf("table %q: primary key column %q not found", t.Name, name)
			}
			if slices.Contains(keys, column.ID) {
				return fmt.Errorf("table %q: primary key column %q is listed twice", t.Name, name)
			}
			keys = append(keys, column.ID)
		}

		for i := range t.Columns {
			if t.Columns[i].PK && !slices.Contains(keys, t.Columns[i].ID) {
				return fmt.Errorf("table %q: column %q is flagged primary_key but the table declares another primary key", t.Name, t.Columns[i].Name)
			}
			t.Columns[i].PK = slices.Contains(keys, t.Columns[i].ID)
		}
	} else {
		for i := range t.Columns {
			if t.Columns[i].PK {
				keys = append(keys, t.Columns[i].ID)
			}
		}
	}

	for _, id := range keys {
		column := &t.Columns[id]
		switch {
		case column.Type.Kind == KindList || column.Type.Kind == KindStruct || column.Type.Kind == KindTimestamp:
			return fmt.Errorf("table %q: primary key column %q is a %s, a key must be a scalar", t.Name, column.Name, column.Type.Kind)
		case column.Optional:
			return fmt.Errorf("table %q: primary key column %q is optional (proto3 optional or oneof member), a key must always be set", t.Name, column.Name)
		}
	}

	switch len(keys) {
	case 0:
		t.PK = PrimaryKey{Kind: PKKindNone}
	case 1:
		t.PK = PrimaryKey{Kind: PKKindSingle, Single: keys[0]}
	default:
		t.PK = PrimaryKey{Kind: PKKindComposite, Composite: keys}
	}
	return nil
}

func (t *Table) columnByNameOrField(name string) *Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name || (t.Columns[i].Field != nil && string(t.Columns[i].Field.Name()) == name) {
			return &t.Columns[i]
		}
	}
	return nil
}

func (t *Table) checkColumns() error {
	seen := make(map[string]struct{}, len(t.Columns))
	for i := range t.Columns {
		if _, dup := seen[t.Columns[i].Name]; dup {
			return fmt.Errorf("table %q: duplicate column name %q", t.Name, t.Columns[i].Name)
		}
		seen[t.Columns[i].Name] = struct{}{}
	}
	return nil
}

// validateRelations checks `child_of` once every table is known. A parent must exist, be keyed by a single
// column, and that column must be the one named by the annotation.
func (r *resolver) validateRelations() error {
	s := r.schema
	for i := range s.Tables {
		t := &s.Tables[i]
		if !t.HasChildOf {
			continue
		}

		parent := s.Table(t.ChildOf.ParentTable)
		if parent == nil {
			return fmt.Errorf("table %q is child_of unknown table %q", t.Name, t.ChildOf.ParentTable)
		}
		if parent.PK.Kind != PKKindSingle {
			return fmt.Errorf("table %q is child_of %q which has no single-column primary key", t.Name, parent.Name)
		}
		if parent.ID == t.ID {
			return fmt.Errorf("table %q is child_of itself", t.Name)
		}
		if pk := &parent.Columns[parent.PK.Single]; pk.Name != t.ChildOf.ParentField && string(pk.Field.Name()) != t.ChildOf.ParentField {
			return fmt.Errorf("table %q is child_of %q on %q but the parent's primary key is %q", t.Name, parent.Name, t.ChildOf.ParentField, pk.Name)
		}
	}
	return nil
}

// reachableTables lists the tables directly under a message, looking through pass-through messages.
func (r *resolver) reachableTables(p *msgPlan) []TableID {
	var out []TableID
	var visit func(*msgPlan)
	visit = func(from *msgPlan) {
		for i := range from.fields {
			child := from.fields[i].child
			if child == nil {
				continue
			}
			if child.hasTbl {
				if !slices.Contains(out, child.tid) {
					out = append(out, child.tid)
				}
				continue
			}
			visit(child)
		}
	}
	visit(p)
	return out
}
