package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMapSlice(t *testing.T) {
	type Boo struct {
		Name string
		Mapz mapSlice
	}
	var b *Boo
	err := yaml.Unmarshal([]byte(`
name: "asldkfj"
mapz:
  amateur: de gruau
  plusieurs: fois
`), &b)
	require.NoError(t, err)

	assert.Equal(t, "asldkfj", b.Name)
	assert.Equal(t, 2, len(b.Mapz))
	assert.Equal(t, "amateur", b.Mapz[0][0])
	assert.Equal(t, "de gruau", b.Mapz[0][1])
	assert.Equal(t, "plusieurs", b.Mapz[1][0])
	assert.Equal(t, "fois", b.Mapz[1][1])
}
