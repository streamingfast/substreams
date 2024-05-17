package sqe

import (
	"bytes"
	"testing"

	lex "github.com/alecthomas/participle/lexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		name   string
		sqe    string
		tokens []string
	}{
		{"minus_followed_by_name", `-token`, []string{"NotOperator", "Name", "EOF"}},

		{"name_with_inside_minus", `open-token`, []string{"Name", "EOF"}},

		// {"legacy_and", `AND`, []string{"AndOperator", "EOF"}},
		{"new_and", `&&`, []string{"AndOperator", "EOF"}},

		// {"legacy_or", `OR`, []string{"OrOperator", "EOF"}},
		{"new_or", `||`, []string{"OrOperator", "EOF"}},

		{"quoting characters start", `'some "some`, []string{"Quoting", "Name", "Space", "Quoting", "Name", "EOF"}},
		{"quoting characters end", `some' some"`, []string{"Name", "Quoting", "Space", "Name", "Quoting", "EOF"}},

		// {"square_brackets", `[field, "double quoted"]`, []string{"LeftSquareBracket", "Name", "Comma", "Space", "Quoting", "Name", "Space", "Name", "Quoting", "RightSquareBracket", "EOF"}},

		{"expresion_with_and", `action && field`, []string{"Name", "Space", "AndOperator", "Space", "Name", "EOF"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := tokensList(t, test.sqe)

			assert.Equal(t, test.tokens, actual)
		})
	}
}

func tokensList(t *testing.T, input string) (out []string) {
	lexer, err := newLexer(bytes.NewBufferString(input))
	require.NoError(t, err)

	tokens, err := lex.ConsumeAll(lexer.PeekingLexer)
	require.NoError(t, err)

	for _, token := range tokens {
		out = append(out, lexer.getTokenType(token))
	}

	return
}
