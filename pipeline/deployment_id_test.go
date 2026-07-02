package pipeline

import "testing"

func TestDeploymentIDFromIdentifier(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"deadbeef@v1.2.3", "deadbeef"},
		{"deadbeef", "deadbeef"},
		{"dead@vbeef@v2", "dead@vbeef"},
		{"@v1.0.0", "@v1.0.0"},
		{"", ""},
	}
	for _, c := range cases {
		if got := deploymentIDFromIdentifier(c.in); got != c.want {
			t.Errorf("deploymentIDFromIdentifier(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
