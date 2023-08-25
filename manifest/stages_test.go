package manifest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStages(t *testing.T) {
	src := []string{
		"M1:B",
		"S1:P,M1",
		"M2:S1",
		"S2:M2",
		"S3:M2",
		"S4:P,M2,C",
		"S5:S4",
		"M3:P,S2,S3,S4,S5,S1",
		"S6:S3,S5",
		"S30:S5",
		"M4:M3,S5,S1,S2,S30",
		"M10:C",
		"S10:M10,M1",
	}
	expect := [][]string{
		{"M1", "M10"},
		{"S1", "S10"},
		{"M2"},
		{"S2", "S3", "S4"},
		{"S5"},
		{"M3"},
		{"S6", "S30"},
		{"M4"},
	}

	seen := map[string]bool{"B": true, "C": true, "P": true}
	origLen := len(seen)
	var stages [][]string

	for i := 0; ; i++ {
		if len(seen)-origLen == len(src) {
			break
		}
		var stage []string
	modLoop:
		for _, modSpec := range src {
			parts := strings.Split(modSpec, ":")
			modName := parts[0]
			deps := strings.Split(parts[1], ",")

			if i%2 == 0 && modName[0] == 'M' {
				continue
			} else if i%2 == 1 && modName[0] == 'S' {
				continue
			}

			if seen[modName] {
				continue
			}

			for _, dep := range deps {
				if !seen[dep] {
					continue modLoop
				}
			}

			stage = append(stage, modName)
		}
		if len(stage) != 0 {
			stages = append(stages, stage)
			for _, mod := range stage {
				seen[mod] = true
			}
		}
	}

	assert.Equal(t, expect, stages)
}
