package manifest

import (
	"testing"

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
