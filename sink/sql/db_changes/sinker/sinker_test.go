package sinker

import (
	"context"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/logging"
	sink "github.com/streamingfast/substreams/sink"
	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

var logger *zap.Logger
var tracer logging.Tracer

func init() {
	logger, tracer = logging.ApplicationLogger("test", "test")
}

func TestSinker_SQLStatements(t *testing.T) {
	tests := []struct {
		name      string
		events    []event
		expectSQL []string
	}{
		{
			name: "insert final block",
			events: []event{
				{
					blockNum:     10,
					libNum:       10,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 10;`,
				`UPDATE "testschema"."cursors" set cursor = 'bN7dsAhRyo44yl_ykkjA36WwLpc_DFtvXwrlIBBBj4r2', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
		{
			name: "insert two final blocks",
			events: []event{
				{
					blockNum:     10,
					libNum:       10,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum:     11,
					libNum:       11,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "2345", "from", "sender2", "to", "receiver2")},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 10;`,
				`UPDATE "testschema"."cursors" set cursor = 'bN7dsAhRyo44yl_ykkjA36WwLpc_DFtvXwrlIBBBj4r2', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender2','2345','receiver2');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 11;`,
				`UPDATE "testschema"."cursors" set cursor = 'dR5-m-1v1TQvlVRfIM9SXaWwLpc_DFtuXwrkIBBAj4r3', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
		{
			name: "insert a reversible blocks",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
		{
			name: "insert, then update",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowMultiplePK("xfer", map[string]string{"id": "1234", "idx": "3"}, "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum: 11,
					libNum:   6,
					tableChanges: []*pbdatabase.TableChange{
						updateRowMultiplePK("xfer", map[string]string{"id": "2345", "idx": "3"}, "from", "sender2", "to", "receiver2"),
					},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234","idx":"3"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,prev_value,block_num) SELECT 'U','"testschema"."xfer"','{"id":"2345","idx":"3"}',row_to_json("xfer"),11 FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3';`,
				`UPDATE "testschema"."xfer" SET "from"='sender2', "to"='receiver2' WHERE "id" = '2345' AND "idx" = '3'`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 6;`,
				`UPDATE "testschema"."cursors" set cursor = 'LamYQ1PoEJyzLTRd7kdEiKWwLpcyB1tlVArvLBtH', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},

		{
			name: "insert, then update, then delete (update disappears)",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowMultiplePK("xfer", map[string]string{"id": "1234", "idx": "3"}, "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum: 11,
					libNum:   6,
					tableChanges: []*pbdatabase.TableChange{
						updateRowMultiplePK("xfer", map[string]string{"id": "2345", "idx": "3"}, "from", "sender2", "to", "receiver2"),
						deleteRowMultiplePK("xfer", map[string]string{"id": "2345", "idx": "3"}),
					},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234","idx":"3"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				// the following gets deduped
				//`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,prev_value,block_num) SELECT 'U','"testschema"."xfer"','{"id":"2345","idx":"3"}',row_to_json("xfer"),11 FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3';` +
				//	`UPDATE "testschema"."xfer" SET "from"='sender2', "to"='receiver2' WHERE "id" = '2345' AND "idx" = '3'`,
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,prev_value,block_num) SELECT 'D','"testschema"."xfer"','{"id":"2345","idx":"3"}',row_to_json("xfer"),11 FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3';`,
				`DELETE FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3'`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 6;`,
				`UPDATE "testschema"."cursors" set cursor = 'LamYQ1PoEJyzLTRd7kdEiKWwLpcyB1tlVArvLBtH', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},

		{
			name: "insert two reversible blocks, then UNDO last",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum:     11,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "2345", "from", "sender2", "to", "receiver2")},
				},
				{
					blockNum:   10, // undo everything above 10
					libNum:     5,
					undoSignal: true,
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"2345"}',11);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender2','2345','receiver2');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'Euaqz6R-ylLG0gbdej7Me6WwLpcyB1tlVArvLxtE', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
				`SELECT op,table_name,pk,prev_value,block_num FROM "testschema"."substreams_history" WHERE "block_num" > 10 ORDER BY "block_num" DESC`,

				//`DELETE FROM "testschema"."xfer" WHERE "id" = "2345";`, // this mechanism is tested in db.revertOp
				`DELETE FROM "testschema"."substreams_history" WHERE "block_num" > 10;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			tx := &db2.TestTx{}
			l := db2.NewTestLoader(
				t,
				"psql://x:5432/x?schemaName=testschema",
				tx,
				db2.TestSinglePrimaryKeyTables("testschema"),
				logger,
				tracer,
			)
			s, err := sink.NewFromConfig(&sink.SinkerConfig{
				Pkg:              testPackage,
				OutputModule:     testPackage.Modules.Modules[0],
				OutputModuleHash: manifest.ModuleHash([]byte("unused")),
				ClientConfig:     testClientConfig,
				Mode:             sink.SubstreamsModeDevelopment,
				NoopMode:         false,
				Logger:           logger,
				Tracer:           nil,
			})
			require.NoError(t, err)
			sinker, _ := New(s, l, logger, nil, 3, 1*time.Second)

			for _, evt := range test.events {
				if evt.undoSignal {
					cursor := simpleCursor(evt.blockNum, evt.libNum)
					err := sinker.HandleBlockUndoSignal(ctx, &pbsubstreamsrpc.BlockUndoSignal{
						LastValidBlock:  &pbsubstreams.BlockRef{Id: fmt.Sprintf("%d", evt.blockNum), Number: evt.blockNum},
						LastValidCursor: cursor,
					}, sink.MustNewCursor(cursor))
					require.NoError(t, err)
					continue
				}

				err := sinker.HandleBlockScopedData(
					ctx,
					blockScopedData("db_out", evt.tableChanges, evt.blockNum, evt.libNum),
					flushEveryBlock, sink.MustNewCursor(simpleCursor(evt.blockNum, evt.libNum)),
				)
				require.NoError(t, err)
			}

			results := tx.Results()
			assert.Equal(t, test.expectSQL, results)
		})
	}
}

type event struct {
	blockNum     uint64
	libNum       uint64
	tableChanges []*pbdatabase.TableChange
	undoSignal   bool
}

var T = true
var flushEveryBlock = &T

var testPackage = &pbsubstreams.Package{
	Modules: &pbsubstreams.Modules{
		Modules: []*pbsubstreams.Module{
			{
				Name: "db_out",
				Kind: &pbsubstreams.Module_KindMap_{},
				Output: &pbsubstreams.Module_Output{
					Type: "proto:sf.substreams.sink.database.v1.DatabaseChanges",
				},
			},
		},
	},
}

var testClientConfig = &client.SubstreamsClientConfig{}

func getFields(fieldsAndValues ...string) (out []*pbdatabase.Field) {
	if len(fieldsAndValues)%2 != 0 {
		panic("tableChangeSinglePK needs even number of fieldsAndValues")
	}
	for i := 0; i < len(fieldsAndValues); i += 2 {
		out = append(out, &pbdatabase.Field{
			Name:  fieldsAndValues[i],
			Value: fieldsAndValues[i+1],
		})
	}
	return
}

func insertRowSinglePK(table string, pk string, fieldsAndValues ...string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_Pk{
			Pk: pk,
		},
		Operation: pbdatabase.TableChange_OPERATION_CREATE,
		Fields:    getFields(fieldsAndValues...),
	}
}

func insertRowMultiplePK(table string, pk map[string]string, fieldsAndValues ...string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_CREATE,
		Fields:    getFields(fieldsAndValues...),
	}
}

func updateRowMultiplePK(table string, pk map[string]string, fieldsAndValues ...string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_UPDATE,
		Fields:    getFields(fieldsAndValues...),
	}
}
func deleteRowMultiplePK(table string, pk map[string]string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_DELETE,
	}
}

func blockScopedData(module string, changes []*pbdatabase.TableChange, blockNum uint64, finalBlockNum uint64) *pbsubstreamsrpc.BlockScopedData {
	mapOutput, err := anypb.New(&pbdatabase.DatabaseChanges{
		TableChanges: changes,
	})
	if err != nil {
		panic(err)
	}

	return &pbsubstreamsrpc.BlockScopedData{
		Output: &pbsubstreamsrpc.MapModuleOutput{
			Name:      module,
			MapOutput: mapOutput,
		},
		Clock:            clock(fmt.Sprintf("%d", blockNum), blockNum),
		Cursor:           simpleCursor(blockNum, finalBlockNum),
		FinalBlockHeight: finalBlockNum,
	}
}

func clock(id string, num uint64) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{Id: id, Number: num}
}

func simpleCursor(num, finalNum uint64) string {
	id := fmt.Sprintf("%d", num)
	finalID := fmt.Sprintf("%d", finalNum)
	blk := bstream.NewBlockRef(id, num)
	lib := bstream.NewBlockRef(finalID, finalNum)
	step := bstream.StepNew
	if id == finalID {
		step = bstream.StepNewIrreversible
	}

	return (&bstream.Cursor{
		Step:      step,
		Block:     blk,
		LIB:       lib,
		HeadBlock: blk,
	}).ToOpaque()
}

// TestProtoUpdateOpToDbUpdateOp tests the proto-to-db UpdateOp converter
func TestProtoUpdateOpToDbUpdateOp(t *testing.T) {
	tests := []struct {
		name     string
		protoOp  pbdatabase.Field_UpdateOp
		expected db2.UpdateOp
	}{
		{"SET (default)", pbdatabase.Field_UPDATE_OP_SET, db2.UpdateOpSet},
		{"ADD", pbdatabase.Field_UPDATE_OP_ADD, db2.UpdateOpAdd},
		{"MAX", pbdatabase.Field_UPDATE_OP_MAX, db2.UpdateOpMax},
		{"MIN", pbdatabase.Field_UPDATE_OP_MIN, db2.UpdateOpMin},
		{"SET_IF_NULL", pbdatabase.Field_UPDATE_OP_SET_IF_NULL, db2.UpdateOpSetIfNull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := protoUpdateOpToDbUpdateOp(tt.protoOp)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProtoUpdateOpToDbUpdateOp_UnknownValue tests that unknown proto values default to SET
func TestProtoUpdateOpToDbUpdateOp_UnknownValue(t *testing.T) {
	// Test with an unknown/invalid proto value - should default to SET
	unknownOp := pbdatabase.Field_UpdateOp(999)
	result := protoUpdateOpToDbUpdateOp(unknownOp)
	assert.Equal(t, db2.UpdateOpSet, result, "unknown proto values should default to SET")
}
