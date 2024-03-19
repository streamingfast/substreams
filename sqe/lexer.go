package sqe

import (
	"fmt"
	"io"

	lex "github.com/alecthomas/participle/lexer"
)

type lexer struct {
	*lex.PeekingLexer

	symbols map[rune]string
}

func newLexer(reader io.Reader) (*lexer, error) {
	l, err := lexerDefinition.Lex(reader)
	if err != nil {
		return nil, fmt.Errorf("new lexer: %s", err)
	}

	peekingLexer, err := lex.Upgrade(l)
	if err != nil {
		return nil, fmt.Errorf("peekable lexer: %s", err)
	}

	return &lexer{
		PeekingLexer: peekingLexer,
		symbols:      lex.SymbolsByRune(lexerDefinition),
	}, nil
}

func (l *lexer) skipSpaces() {
	for {
		token, err := l.Peek(0)
		if err != nil || !l.isSpace(token) {
			return
		}

		l.mustLexNext()
	}
}

func (l *lexer) mustLexNext() lex.Token {
	token, err := l.Next()
	if err != nil {
		panic(err)
	}

	return token
}

func (l *lexer) peekPos() lex.Position {
	peek, err := l.Peek(0)
	if err != nil {
		return lex.Position{Filename: "", Line: 1, Offset: l.PeekingLexer.Length() - 1, Column: l.PeekingLexer.Length()}
	}

	return peek.Pos
}

var lexerDefinition = lex.Must(lex.Regexp(
	`(?m)` +
		`(?P<Quoting>"|')` +
		// `|(?P<Comma>,)` +
		`|(?P<NotOperator>\-)` +
		`|(?P<OrOperator>\|\|)` +
		`|(?P<AndOperator>&&)` +
		`|(?P<LeftParenthesis>\()` +
		`|(?P<RightParenthesis>\))` +
		// `|(?P<LeftSquareBracket>\[)` +
		// `|(?P<RightSquareBracket>\])` +
		`|(?P<Name>[^\s'"\-\(\)][^\s'"\(\)]*)` +
		`|(?P<Space>\s+)`,
))

func (l *lexer) isSpace(t lex.Token) bool   { return l.isTokenType(t, "Space") }
func (l *lexer) isQuoting(t lex.Token) bool { return l.isTokenType(t, "Quoting") }

// func (l *lexer) isColon(t lex.Token) bool              { return l.isTokenType(t, "Colon") }
// func (l *lexer) isComma(t lex.Token) bool              { return l.isTokenType(t, "Comma") }
func (l *lexer) isNotOperator(t lex.Token) bool      { return l.isTokenType(t, "NotOperator") }
func (l *lexer) isOrOperator(t lex.Token) bool       { return l.isTokenType(t, "OrOperator") }
func (l *lexer) isAndOperator(t lex.Token) bool      { return l.isTokenType(t, "AndOperator") }
func (l *lexer) isLeftParenthesis(t lex.Token) bool  { return l.isTokenType(t, "LeftParenthesis") }
func (l *lexer) isRightParenthesis(t lex.Token) bool { return l.isTokenType(t, "RightParenthesis") }

// func (l *lexer) isLeftSquareBracket(t lex.Token) bool  { return l.isTokenType(t, "LeftSquareBracket") }
// func (l *lexer) isRightSquareBracket(t lex.Token) bool { return l.isTokenType(t, "RightSquareBracket") }
func (l *lexer) isName(t lex.Token) bool { return l.isTokenType(t, "Name") }

func (l *lexer) isTokenType(token lex.Token, expectedType string) bool {
	return l.symbols[token.Type] == expectedType
}

func (l *lexer) isBinaryOperator(t lex.Token) bool {
	return l.isAnyTokenType(t, "AndOperator", "OrOperator")
}

func (l *lexer) isAnyTokenType(token lex.Token, expectedTypes ...string) bool {
	for _, expectedType := range expectedTypes {
		if l.symbols[token.Type] == expectedType {
			return true
		}
	}
	return false
}

func (l *lexer) getTokenType(token lex.Token) string {
	return l.symbols[token.Type]
}
