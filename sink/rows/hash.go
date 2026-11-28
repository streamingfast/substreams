package rows

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"strings"
)

// canonical is the text form the hash is computed over, and what [Schema.Describe] prints after its header. Tables
// are sorted by name and columns keep their order, so it does not depend on map order, protobuf field numbers or any
// SQL dialect.
func (s *Schema) canonical() string {
	order := make([]int, len(s.Tables))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return strings.Compare(s.Tables[a].Name, s.Tables[b].Name) })

	var sb strings.Builder
	for _, i := range order {
		t := &s.Tables[i]

		fmt.Fprintf(&sb, "table %s", t.Name)
		if t.PK.Len() > 0 {
			names := make([]string, t.PK.Len())
			for k := range names {
				names[k] = t.Columns[t.PK.At(k)].Name
			}
			fmt.Fprintf(&sb, " (pk %s)", strings.Join(names, ","))
		}
		if t.HasChildOf {
			fmt.Fprintf(&sb, " (child_of %s on %s)", t.ChildOf.ParentTable, t.ChildOf.ParentField)
		}
		sb.WriteByte('\n')

		for k := range t.Columns {
			c := &t.Columns[k]
			fmt.Fprintf(&sb, "  %s %s", c.Name, c.Type)
			if c.Optional {
				sb.WriteString(" optional")
			}
			if c.PK {
				sb.WriteString(" pk")
			}
			if c.Unique {
				sb.WriteString(" unique")
			}
			if c.HasFK {
				fmt.Fprintf(&sb, " fk=%s.%s", c.FK.Table, c.FK.Field)
			}
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// Hash identifies the schema: the first 16 bytes of the SHA-256 of its canonical text, hex encoded. It covers names,
// logical types, optionality, keys, unique and foreign key flags and `child_of`, so a rename or a `convertTo` change
// flips it. It is independent of table order and of everything SQL (it is not the db_proto schema hash).
func (s *Schema) Hash() string {
	sum := sha256.Sum256([]byte(s.canonical()))
	return hex.EncodeToString(sum[:16])
}

// Describe writes the canonical text of the schema, preceded by its hash.
func (s *Schema) Describe(w io.Writer) error {
	_, err := fmt.Fprintf(w, "schema %s\n%s", s.Hash(), s.canonical())
	return err
}
