package sql

import "strconv"

// EnumValue carries a protobuf enum field through the walk, keeping both renderings.
//
// The dialects disagree on how an enum should be stored — PostgreSQL declares the column
// TEXT, ClickHouse declares it Int32 — and the walk is shared, so it cannot pick one.
// Passing both lets each dialect take what matches the column it created, and neither
// has to reach back to the field descriptor to resolve a name.
type EnumValue struct {
	Number int32
	Name   string
}

// String is the textual rendering: the enum's name, or its number when the value is not
// one the descriptor knows about, which is what a proto3 reader must tolerate.
func (v EnumValue) String() string {
	if v.Name != "" {
		return v.Name
	}

	return strconv.FormatInt(int64(v.Number), 10)
}
