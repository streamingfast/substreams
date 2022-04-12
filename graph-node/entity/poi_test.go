package entity

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

type TestEntity struct {
	Base
	String      string           `db:"string"`
	Integer     Int              `db:"integer"`
	Float       Float            `db:"float"`
	StringPtr   *string          `db:"string_ptr,nullable"`
	FloatPtr    *Float           `db:"float_ptr,nullable" `
	IntegerPtr  *Int             `db:"integer_ptr,nullable" `
	StringArray LocalStringArray `db:"string_array,nullable"`
}

func TestPOI_Write(t *testing.T) {
	stringPtr := "helloworld"
	intPtr := NewInt(new(big.Int).SetUint64(3876123))
	floatPtr := NewFloat(new(big.Float).SetFloat64(1823.231))
	id := "0xb9afd8521c76c56ed4bc12c127c75f2fa9a9f2edda1468138664d4f0c324d30b"

	tests := []struct {
		name       string
		poiID      string
		entityType string
		entityId   string
		entity     *TestEntity
	}{
		{
			name:       "basic-entity",
			poiID:      "test",
			entityType: "test_entities",
			entityId:   "0x7bef660b110023fd795d101d5d63972a82438661",
			entity: &TestEntity{
				Base:        Base{ID: id},
				String:      "",
				Integer:     NewIntFromLiteral(0),
				Float:       NewFloatFromLiteral(0),
				StringPtr:   &stringPtr,
				FloatPtr:    &floatPtr,
				IntegerPtr:  &intPtr,
				StringArray: []string{"aa", "bb", "cc"},
			},
		},
		{
			name:       "entity with nill value",
			poiID:      "test",
			entityType: "test_entities",
			entityId:   "0x7bef660b110023fd795d101d5d63972a82438661",
			entity: &TestEntity{
				Base:    Base{ID: id},
				String:  "",
				Integer: NewIntFromLiteral(0),
				Float:   NewFloatFromLiteral(0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			poi := NewPOI(test.poiID)
			err := poi.AddEnt(test.entityType, test.entity)
			require.NoError(t, err)
			poi.Apply()
		})
	}
}
