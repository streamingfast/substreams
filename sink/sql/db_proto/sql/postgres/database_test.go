package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestConstraintTarget pins how a constraint is identified out of the dialect's own DDL.
//
// Relation as well as name, because names are only unique per table: every table carries
// a foreign key called fk_block, so a name on its own says a constraint present on one
// table is present on all of them — which would skip creating the rest and drop only one.
func TestConstraintTarget(t *testing.T) {
	tests := []struct {
		name         string
		statement    string
		expectFound  bool
		expectRelate string
		expectName   string
	}{
		{
			"primary key, as the dialect writes it",
			`alter table myschema.balancechange add constraint balancechange_pk primary key ("id");`,
			true, "myschema.balancechange", "balancechange_pk",
		},
		{
			"the block table's primary key",
			`alter table myschema._blocks_ add constraint block_pk primary key (number);`,
			true, "myschema._blocks_", "block_pk",
		},
		{
			"unique constraint",
			`alter table myschema.transfer add constraint transfer_hash_unique unique ("hash");`,
			true, "myschema.transfer", "transfer_hash_unique",
		},
		{
			// ForeignKey.String() puts two spaces after the name, and upper-cases the DDL.
			"foreign key",
			`ALTER TABLE myschema.transfer ADD CONSTRAINT transfer_block_fk  FOREIGN KEY (_block_number_) REFERENCES myschema._blocks_(number)`,
			true, "myschema.transfer", "transfer_block_fk",
		},
		{
			"quoted name and relation are folded the way the server folds them",
			`ALTER TABLE "MySchema"."Transfer" ADD CONSTRAINT "fk_block" FOREIGN KEY (a) REFERENCES b(c)`,
			true, "myschema.transfer", "fk_block",
		},
		{
			// Nothing to skip on: the statement gets executed and the server decides.
			"not a named constraint",
			`CREATE TABLE IF NOT EXISTS myschema.transfer (id text)`,
			false, "", "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, found := constraintTarget(test.statement)

			assert.Equal(t, test.expectFound, found)
			assert.Equal(t, test.expectRelate, key.relation)
			assert.Equal(t, test.expectName, key.name)
		})
	}
}

// TestConstraintTargetDistinguishesTables is the case that made keying by name alone a
// bug: the same constraint name on two tables has to be two different constraints.
func TestConstraintTargetDistinguishesTables(t *testing.T) {
	customers, _ := constraintTarget(`ALTER TABLE myschema.customers ADD CONSTRAINT fk_block FOREIGN KEY (a) REFERENCES b(c)`)
	orders, _ := constraintTarget(`ALTER TABLE myschema.orders ADD CONSTRAINT fk_block FOREIGN KEY (a) REFERENCES b(c)`)

	assert.NotEqual(t, customers, orders)
	assert.Equal(t, customers.name, orders.name)
}
