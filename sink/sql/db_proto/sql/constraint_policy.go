package sql

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// AllTables is what a per-table constraint switch takes to mean every table at once.
const AllTables = "all"

// ConstraintPolicy says which constraints the schema gets and when.
//
// Constraints are created after the load rather than during it — measured on 500k rows
// through binary COPY, loading with foreign keys in place costs 27.7x, while building the
// same constraints after the load costs 3.3x, for an identical schema. See
// TestConstraintCost in sink/sql/db_proto/benchmarks.

// ConstraintTiming says when the constraints are created.
type ConstraintTiming string

const (
	// ConstraintsAuto has the sink create them itself once the backfill is over: at the
	// first live block, or at the end of a bounded run. It is the default, stop-the-world
	// pass and all. A backfill that ends with no primary keys and no foreign keys has
	// produced a database nobody should query, and leaving it that way until the operator
	// remembers a second command is the worse failure — it is silent, and it looks like
	// success.
	ConstraintsAuto ConstraintTiming = "auto"

	// ConstraintsManual leaves it to the operator, through `sink postgres constraints
	// apply`. Building them locks every table while indexes are built and every foreign
	// key is validated, so on a large database this is how the pass goes into a
	// maintenance window instead.
	ConstraintsManual ConstraintTiming = "manual"

	// ConstraintsAlways creates them before the first row is written, so the database
	// rejects bad data from the start and the load pays for it throughout.
	ConstraintsAlways ConstraintTiming = "always"
)

type ConstraintPolicy struct {
	// Timing decides when they are created; the zero value is ConstraintsAuto.
	Timing ConstraintTiming

	// DisableForeignKeys leaves out every foreign key, including the one to the block
	// table. Those are what a load pays most for.
	DisableForeignKeys bool

	// DisablePrimaryKeys and DisableUniques name the tables that go without, or AllTables.
	DisablePrimaryKeys []string
	DisableUniques     []string
}

// SkipPrimaryKey reports whether the given table is meant to go without its primary key.
func (p ConstraintPolicy) SkipPrimaryKey(table string) bool {
	return matchesTable(p.DisablePrimaryKeys, table)
}

// SkipUnique reports whether the given table's unique constraints are meant to be left out.
func (p ConstraintPolicy) SkipUnique(table string) bool {
	return matchesTable(p.DisableUniques, table)
}

// SkipForeignKey reports whether the given table's foreign keys are meant to be left out.
func (p ConstraintPolicy) SkipForeignKey(string) bool {
	return p.DisableForeignKeys
}

// SkipsEverything reports a policy that leaves the schema with no constraints at all, in
// which case there is nothing to apply and nothing to wait for.
func (p ConstraintPolicy) SkipsEverything() bool {
	return p.DisableForeignKeys && matchesTable(p.DisablePrimaryKeys, AllTables) && matchesTable(p.DisableUniques, AllTables)
}

// DisableAllConstraints returns the policy that creates none of them, which is what a
// pure bulk load into a throwaway database wants.
func DisableAllConstraints() ConstraintPolicy {
	return ConstraintPolicy{
		DisableForeignKeys: true,
		DisablePrimaryKeys: []string{AllTables},
		DisableUniques:     []string{AllTables},
	}
}

// Describe renders the policy for a log line.
func (p ConstraintPolicy) Describe() string {
	if p.SkipsEverything() {
		return "none"
	}

	var parts []string
	if p.DisableForeignKeys {
		parts = append(parts, "no foreign keys")
	}
	if len(p.DisablePrimaryKeys) > 0 {
		parts = append(parts, fmt.Sprintf("no primary key on %s", strings.Join(p.DisablePrimaryKeys, ",")))
	}
	if len(p.DisableUniques) > 0 {
		parts = append(parts, fmt.Sprintf("no unique constraint on %s", strings.Join(p.DisableUniques, ",")))
	}
	if len(parts) == 0 {
		parts = append(parts, "all")
	}

	when := "created once the backfill is done"
	switch p.Timing {
	case ConstraintsAlways:
		when = "created before the load"
	case ConstraintsManual:
		when = "created by `sink postgres constraints apply`"
	}

	return strings.Join(parts, ", ") + ", " + when
}

func matchesTable(list []string, table string) bool {
	for _, entry := range list {
		entry = strings.TrimSpace(entry)
		if strings.EqualFold(entry, AllTables) || strings.EqualFold(entry, table) {
			return true
		}
	}

	return false
}

// ApplyUpfront reports whether the constraints go in before the first row.
func (p ConstraintPolicy) ApplyUpfront() bool {
	return p.Timing == ConstraintsAlways && !p.SkipsEverything()
}

// ApplyAtHead reports whether the sink creates them itself once the backfill is over.
func (p ConstraintPolicy) ApplyAtHead() bool {
	return (p.Timing == ConstraintsAuto || p.Timing == "") && !p.SkipsEverything()
}

// storedPolicy is what gets recorded, which is the shape of the schema and not the
// timing: when constraints are created is a decision each run makes, which tables are
// meant to have them is a property of the schema every command has to agree on.
type storedPolicy struct {
	DisableForeignKeys bool     `json:"disable_foreign_keys"`
	DisablePrimaryKeys []string `json:"disable_primary_keys,omitempty"`
	DisableUniques     []string `json:"disable_uniques,omitempty"`
}

// Encode renders the schema-shaping half of the policy for storage.
func (p ConstraintPolicy) Encode() (string, error) {
	encoded, err := json.Marshal(storedPolicy{
		DisableForeignKeys: p.DisableForeignKeys,
		DisablePrimaryKeys: p.DisablePrimaryKeys,
		DisableUniques:     p.DisableUniques,
	})
	if err != nil {
		return "", fmt.Errorf("encoding the constraint policy: %w", err)
	}

	return string(encoded), nil
}

// DecodeConstraintPolicy reads back what Encode wrote, keeping the caller's timing.
func DecodeConstraintPolicy(encoded string, timing ConstraintTiming) (ConstraintPolicy, error) {
	var stored storedPolicy
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		return ConstraintPolicy{}, fmt.Errorf("decoding the recorded constraint policy %q: %w", encoded, err)
	}

	return ConstraintPolicy{
		Timing:             timing,
		DisableForeignKeys: stored.DisableForeignKeys,
		DisablePrimaryKeys: stored.DisablePrimaryKeys,
		DisableUniques:     stored.DisableUniques,
	}, nil
}

// SameShape reports whether two policies describe the same schema, ignoring timing.
func (p ConstraintPolicy) SameShape(other ConstraintPolicy) bool {
	return p.DisableForeignKeys == other.DisableForeignKeys &&
		slices.Equal(normalizeTables(p.DisablePrimaryKeys), normalizeTables(other.DisablePrimaryKeys)) &&
		slices.Equal(normalizeTables(p.DisableUniques), normalizeTables(other.DisableUniques))
}

func normalizeTables(list []string) []string {
	out := make([]string, 0, len(list))
	for _, entry := range list {
		if trimmed := strings.ToLower(strings.TrimSpace(entry)); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	slices.Sort(out)

	return out
}

// ParseConstraintTiming validates the flag value.
func ParseConstraintTiming(in string) (ConstraintTiming, error) {
	switch ConstraintTiming(in) {
	case "", ConstraintsAuto:
		return ConstraintsAuto, nil
	case ConstraintsManual:
		return ConstraintsManual, nil
	case ConstraintsAlways:
		return ConstraintsAlways, nil
	}

	return "", fmt.Errorf("invalid constraint timing %q, expected one of %q, %q or %q", in, ConstraintsAuto, ConstraintsManual, ConstraintsAlways)
}
