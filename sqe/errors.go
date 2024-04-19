package sqe

import (
	"fmt"

	lex "github.com/alecthomas/participle/lexer"
)

type ParseError struct {
	message  string
	position lex.Position
}

func parserError(message string, position lex.Position) *ParseError {
	return &ParseError{
		message:  message,
		position: position,
	}
}

func rangeParserError(message string, start lex.Position, end lex.Position) *ParseError {
	return &ParseError{
		message: message,
		position: lex.Position{
			Filename: start.Filename,
			Offset:   start.Offset,
			Line:     start.Line,
			Column:   end.Column,
		},
	}
}

func (e *ParseError) Error() string {
	if e.position.Line <= 1 {
		return fmt.Sprintf("%s at column %d", e.message, e.position.Offset)
	}

	return fmt.Sprintf("%s at line %d column %d", e.message, e.position.Line, e.position.Column)
}
