package manifest

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGraph(t *testing.T) {
	man, err := New("./test/test_manifest.yaml")
	assert.NoError(t, err)

	x, _ := man.Graph.AncestorsOf("reserves_extractor")
	fmt.Println(x)
	x, _ = man.Graph.ModulesDownTo("reserves_extractor")
	fmt.Println(x)
	xy, _ := man.Graph.GroupedModulesDownTo("reserves_extractor")
	fmt.Println(xy)
}
