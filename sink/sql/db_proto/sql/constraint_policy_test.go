package sql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConstraintPolicyRoundTrip pins what gets recorded at setup and read back by
// `constraints apply`: the shape of the schema, not the timing. Timing is a decision each
// run makes; which tables are meant to carry constraints is a property of the schema every
// command has to agree on.
func TestConstraintPolicyRoundTrip(t *testing.T) {
	policy := ConstraintPolicy{
		Timing:             ConstraintsAlways,
		DisableForeignKeys: true,
		DisablePrimaryKeys: []string{"orders", "order_items"},
		DisableUniques:     []string{AllTables},
	}

	encoded, err := policy.Encode()
	require.NoError(t, err)

	decoded, err := DecodeConstraintPolicy(encoded, ConstraintsManual)
	require.NoError(t, err)

	require.Equal(t, ConstraintsManual, decoded.Timing, "timing comes from the caller, not from the record")
	require.True(t, policy.SameShape(decoded))
}

func TestConstraintPolicySameShape(t *testing.T) {
	base := ConstraintPolicy{DisablePrimaryKeys: []string{"orders", "customers"}}

	require.True(t, base.SameShape(ConstraintPolicy{DisablePrimaryKeys: []string{" Customers ", "ORDERS"}}),
		"order, case and surrounding space are not a disagreement")
	require.False(t, base.SameShape(ConstraintPolicy{DisablePrimaryKeys: []string{"orders"}}))
	require.False(t, base.SameShape(ConstraintPolicy{DisablePrimaryKeys: []string{"orders", "customers"}, DisableForeignKeys: true}))
}
