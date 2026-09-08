package manifest

import (
	"os"
	"path/filepath"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

// The SQL sink Service descriptors must be resolvable from the bundled system
// descriptors (pb/system/system.pb): a manifest with a `sink:` block of one of
// these types fails to parse otherwise.
func TestSystemProtobufs_ResolveSQLSinkServiceTypes(t *testing.T) {
	fds, err := readSystemProtobufs()
	require.NoError(t, err)

	for _, typ := range []string{
		"sf.substreams.sink.sql.v1.Service",
		"sf.substreams.sink.sql.service.v1.Service",
	} {
		msgDesc, err := getMsgDesc(typ, fds.File)
		require.NoError(t, err, "type %s not in bundled system descriptors", typ)
		require.NotNil(t, msgDesc)
	}
}

// A manifest that imports the SQL sink schema must parse without the file being
// on disk. It is a system protobuf, but protoparse needs the source to honour
// its extensions, so it is served from an embedded copy.
func TestSQLSchemaProto_ResolvesWithoutLocalCopy(t *testing.T) {
	dir := t.TempDir()
	protoDir := filepath.Join(dir, "proto")
	require.NoError(t, os.MkdirAll(protoDir, 0o755))

	contract := `syntax = "proto3";
package test.annotated;
import "sf/substreams/sink/sql/schema/v1/schema.proto";
message Row {
  option (schema.table) = {
    name: "Row"
    clickhouse_table_options: { order_by_fields: [{name: "id"}] }
  };
  string id = 1 [(schema.field) = { primary_key: true }];
}`
	require.NoError(t, os.WriteFile(filepath.Join(protoDir, "contract.proto"), []byte(contract), 0o644))

	pkg := &pbsubstreams.Package{}
	_, err := loadProtobufFromDirectory(pkg, protoDir)
	require.NoError(t, err)
}
