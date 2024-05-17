package sqe

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	lex "github.com/alecthomas/participle/lexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ValidateOnlyThatItParses = "!__valiateOnlyThatItParses__!"

func TestParser(t *testing.T) {
	tests := []struct {
		name        string
		sqe         string
		expected    string
		expectedErr error
	}{
		{
			"single_key_term",
			`transfer`,
			`transfer`,
			nil,
		},
		{
			"single_key_term_space_before",
			` transfer`,
			`transfer`,
			nil,
		},
		{
			"single_key_term_space_after",
			`transfer `,
			`transfer`,
			nil,
		},
		{
			"single_key_term_space_both",
			` transfer `,
			`transfer`,
			nil,
		},
		{
			"single_key_term_with_dot_in_it",
			`some.action`,
			`some.action`,
			nil,
		},
		{
			"single_key_term_multi_spaces",
			"  \t transfer",
			`transfer`,
			nil,
		},
		{
			"single_key_term_with_dot",
			`data.name`,
			`data.name`,
			nil,
		},
		{
			"double_quoted_string",
			`"test || value AND other   	( 10 )!"`,
			`"test || value AND other   	( 10 )!"`,
			nil,
		},
		{
			"double_quoted_string_multi_spaces",
			`   "  test || value AND other   	( 10 )!"`,
			`"  test || value AND other   	( 10 )!"`,
			nil,
		},

		{
			"single_quoted_string",
			`'test:value || value AND other   	( 10 )!'`,
			`'test:value || value AND other   	( 10 )!'`,
			nil,
		},
		{
			"single_quoted_string_multi_spaces",
			`   '  test:value || value AND other   	( 10 )!'`,
			`'  test:value || value AND other   	( 10 )!'`,
			nil,
		},

		{
			"top_level_single_and_implicit",
			`one two`,
			"<one && two>",
			nil,
		},
		{
			"top_level_single_and_implicit_double_quotes",
			`"one" two`,
			`<"one" && two>`,
			nil,
		},
		{
			"top_level_single_and",
			`one && two`,
			"<one && two>",
			nil,
		},
		{
			"top_level_single_and_legacy",
			`one && two`,
			"<one && two>",
			nil,
		},

		{
			"top_level_single_or",
			`one || two`,
			"[one || two]",
			nil,
		},
		{
			"top_level_single_or_legacy",
			`one || two`,
			"[one || two]",
			nil,
		},

		{
			"top_level_parenthesis_single_term",
			`(one)`,
			`(one)`,
			nil,
		},
		{
			"top_level_parenthesis_and_term",
			`(one && two)`,
			`(<one && two>)`,
			nil,
		},
		{
			"top_level_parenthesis_and_term_double_quote",
			`(one && "two")`,
			`(<one && "two">)`,
			nil,
		},
		{
			"top_level_parenthesis_or_term",
			`(one || two)`,
			`([one || two])`,
			nil,
		},
		{
			"top_level_parenthesis_or_term_with_double_quotes",
			`(  "one"   || two)`,
			`(["one" || two])`,
			nil,
		},
		{
			"top_level_parenthesis_with_spaces",
			` ( one || two   )  `,
			`([one || two])`,
			nil,
		},

		{
			"top_level_multi_and",
			`a b c d`,
			`<a && b && c && d>`,
			nil,
		},
		{
			"top_level_multi_or",
			`a || b || c || d`,
			`[a || b || c || d]`,
			nil,
		},

		{
			"precedence_and_or",
			`a b || c`,
			`[<a && b> || c]`,
			nil,
		},
		{
			"precedence_or_and",
			`a || b c`,
			`[a || <b && c>]`,
			nil,
		},
		{
			"precedence_and_or_and",
			`a b || c d`,
			`[<a && b> || <c && d>]`,
			nil,
		},
		{
			"precedence_and_and_or",
			`a b c || d`,
			`[<a && b && c> || d]`,
			nil,
		},

		{
			"precedence_parenthesis_and_or_and",
			`a (b || c) d`,
			`<a && ([b || c]) && d>`,
			nil,
		},
		{
			"precedence_parenthesis_and_or",
			`a (b || c)`,
			`<a && ([b || c])>`,
			nil,
		},

		{
			"ported_big_example",
			`"eos" (transfer || issue || matant) from to`,
			`<"eos" && ([transfer || issue || matant]) && from && to>`,
			nil,
		},
		{
			"ported_with_newlines",
			"(a ||\n b)",
			`([a || b])`,
			nil,
		},

		{
			"depthness_100_ors",
			buildFromOrToList(100),
			ValidateOnlyThatItParses,
			nil,
		},
		{
			"depthness_1_000_ors",
			buildFromOrToList(1000),
			ValidateOnlyThatItParses,
			nil,
		},
		{
			"depthness_2_500_ors",
			buildFromOrToList(2500),
			ValidateOnlyThatItParses,
			nil,
		},

		{
			"error_missing_expression_after_and",
			`a && `,
			"",
			fmt.Errorf("missing expression after 'and' clause: %w",
				&ParseError{"expected a key term, minus sign or left parenthesis, got end of input", pos(1, 5, 6)},
			),
		},
		{
			"error_missing_expression_after_or",
			`a || `,
			"",
			fmt.Errorf("missing expression after 'or' clause: %w", &ParseError{"expected a key term, minus sign or left parenthesis, got end of input", pos(1, 5, 6)}),
		},
		{
			"error_unstarted_right_parenthesis",
			`a )`,
			"",
			&ParseError{"unexpected right parenthesis, expected right hand side expression or end of input", pos(1, 2, 3)},
		},
		{
			"error_unclosed_over_left_parenthesis",
			`( a`,
			"",
			&ParseError{"expecting closing parenthesis, got end of input", pos(1, 0, 1)},
		},
		{
			"error_deepness_reached",
			buildFromOrToList(MaxRecursionDeepness + 1),
			"",
			&ParseError{"expression is too long, too much ORs or parenthesis expressions", pos(1, 91251, 91252)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if os.Getenv("DEBUG") != "" {
				printTokens(t, test.sqe)
			}

			parser, err := NewParser(strings.NewReader(test.sqe))
			require.NoError(t, err)

			expression, err := parser.Parse(context.Background())
			require.Equal(t, test.expectedErr, err)

			if test.expectedErr == nil && err == nil && test.expected != ValidateOnlyThatItParses {
				assert.Equal(t, test.expected, expressionToString(expression), "Invalid parsing for SEQ %q", test.sqe)
			}
		})
	}
}

func pos(line, offset, column int) lex.Position {
	return lex.Position{Filename: "", Line: line, Offset: offset, Column: column}
}

func printTokens(t *testing.T, input string) {
	lexer, err := lexerDefinition.Lex(strings.NewReader(input))
	require.NoError(t, err)

	tokens, err := lex.ConsumeAll(lexer)
	require.NoError(t, err)

	for _, token := range tokens {
		fmt.Print(token.GoString())
	}
}
