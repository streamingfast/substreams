package block

import "testing"

func TestParseRangeRejectsExtraBounds(t *testing.T) {
	for _, input := range []string{"10:20:30", "10-20-30", "10:20:30:40"} {
		r, err := ParseRange(input, 100)
		if err == nil || r != nil {
			t.Errorf("ParseRange(%q, 100) = %v, %v; want nil and an error", input, r, err)
		}
	}
}
