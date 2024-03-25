package sqe

import (
	"context"
	"strings"
	"testing"
)

func BenchmarkParseExpression(b *testing.B) {
	tests := []struct {
		name string
		sqe  string
	}{
		{"single term", "action:data"},

		// Those are kind of standard query that are parsed quite often
		{"triple and term", "eosio data specificacct"},
		{"multiple and term", "data data.from: 'action' string"},
		{"multiple and/or term", "data (data.from || data.from) ('action' || expected) 'action' string"},

		// Some convoluted big ORs list
		{"big ORs list 100", buildFromOrToList(100)},
		{"big ORs list 1_000", buildFromOrToList(1000)},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			setupBench(b)
			for n := 0; n < b.N; n++ {
				_, err := Parse(context.Background(), test.sqe)
				if err != nil {
					b.Error(err)
					b.FailNow()
				}
			}
		})
	}
}

func buildFromOrToList(count int) string {
	var elements []string

	// The count is divided by 2 since we add 2 addresses per iteration
	for i := 0; i < count/2; i++ {
		elements = append(elements, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	}

	return "(" + strings.Join(elements, " || ") + ")"
}

func setupBench(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
}
