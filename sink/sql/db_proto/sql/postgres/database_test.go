package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstraintName(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		expect    string
	}{
		{
			"primary key, as the dialect writes it",
			`alter table myschema.balancechange add constraint balancechange_pk primary key ("id");`,
			"balancechange_pk",
		},
		{
			"the block table's primary key",
			`alter table myschema._blocks_ add constraint block_pk primary key (number);`,
			"block_pk",
		},
		{
			"unique constraint",
			`alter table myschema.transfer add constraint transfer_hash_unique unique ("hash");`,
			"transfer_hash_unique",
		},
		{
			// ForeignKey.String() puts two spaces after the name, and upper-cases the DDL.
			"foreign key",
			`ALTER TABLE myschema.transfer ADD CONSTRAINT transfer_block_fk  FOREIGN KEY (_block_number_) REFERENCES myschema._blocks_(number)`,
			"transfer_block_fk",
		},
		{
			"quoted name",
			`ALTER TABLE myschema.transfer ADD CONSTRAINT "fk_block" FOREIGN KEY (a) REFERENCES b(c)`,
			"fk_block",
		},
		{
			// Nothing to skip on: the statement gets executed and the server decides.
			"not a named constraint",
			`CREATE TABLE IF NOT EXISTS myschema.transfer (id text)`,
			"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, constraintName(test.statement))
		})
	}
}
