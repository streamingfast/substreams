package entity

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type PancakeFactory struct {
	Base
	TotalTransactions Int    `db:"total_transactions"`
	TotalVolumeUSD    Float  `db:"total_volume_usd"`
	TotalLiquidityUSD *Float `db:"total_liquidity_usd,nullable"`
}

func TestFieldTags(t *testing.T) {
	tests := []struct {
		name   string
		input  Interface
		expect []*FieldTag
	}{
		{
			name:  "first",
			input: &PancakeFactory{},
			expect: []*FieldTag{
				{Name: "ID", Base: true, ColumnName: "id"},
				{Name: "VID", Base: true, ColumnName: "vid"},
				{Name: "BlockRange", Base: true, ColumnName: "block_range"},
				{Name: "UpdatedBlockNum", Base: true, ColumnName: "_updated_block_number"},
				{Name: "TotalTransactions", ColumnName: "total_transactions"},
				{Name: "TotalVolumeUSD", ColumnName: "total_volume_usd"},
				{Name: "TotalLiquidityUSD", ColumnName: "total_liquidity_usd", Optional: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := DBFields(reflect.TypeOf(test.input))
			assert.Equal(t, test.expect, res)
		})
	}
}
