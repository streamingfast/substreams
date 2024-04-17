package sqe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	lex "github.com/alecthomas/participle/lexer"
)

// MaxRecursionDeepness is the limit we impose on the number of direct ORs expression.
// It's possible to have more than that, just not in a single successive sequence or `1 or 2 or 3 ...`.
// This is to avoid first a speed problem where parsing start to be
const MaxRecursionDeepness = 2501

func Parse(ctx context.Context, input string) (expr Expression, err error) {
	parser, err := NewParser(bytes.NewBufferString(input))
	if err != nil {
		return nil, fmt.Errorf("new parser: %w", err)
	}

	return parser.Parse(ctx)
}

type Parser struct {
	ctx context.Context
	l   *lexer

	lookForRightParenthesis uint
}

func NewParser(reader io.Reader) (*Parser, error) {
	lexer, err := newLexer(reader)
	if err != nil {
		return nil, err
	}

	return &Parser{
		ctx: context.Background(),
		l:   lexer,
	}, nil
}

func (p *Parser) Parse(ctx context.Context) (out Expression, err error) {
	defer func() {
		recoveredErr := recover()
		if recoveredErr == nil {
			return
		}

		switch v := recoveredErr.(type) {
		case *ParseError:
			err = v
		case error:
			err = fmt.Errorf("unexpected error occurred while parsing SQE expression: %w", v)
		case string, fmt.Stringer:
			err = fmt.Errorf("unexpected error occurred while parsing SQE expression: %s", v)
		default:
			err = fmt.Errorf("unexpected error occurred while parsing SQE expression: %v", v)
		}
	}()

	rootExpr, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}

	return optimizeExpression(ctx, rootExpr), nil
}

func (p *Parser) parseExpression(depth int) (Expression, error) {
	if depth >= MaxRecursionDeepness {
		// This is a small hack, the panic is trapped at the public API `Parse` method. We do it with a panic
		// to avoid the really deep wrapping of error that would happen if we returned right away. A test ensure
		// that this behavior works as expected.
		panic(parserError("expression is too long, too much ORs or parenthesis expressions", p.l.peekPos()))
	}

	left, err := p.parseUnaryExpression(depth)
	if err != nil {
		return nil, err
	}

	for {
		p.l.skipSpaces()
		next, err := p.l.Peek(0)
		if err != nil {
			return nil, err
		}

		// If we reached end of file, we have finished our job
		if next.EOF() {
			return left, nil
		}

		// If we reached right parenthesis, check if we were expecting one
		if p.l.isRightParenthesis(next) {
			if p.lookForRightParenthesis == 0 {
				return nil, parserError("unexpected right parenthesis, expected right hand side expression or end of input", next.Pos)
			}

			// We were expecting one, we finished our job for this part, decrement will be done at parsing site
			return left, nil
		}

		isImplicitAnd := true
		if p.l.isBinaryOperator(next) {
			isImplicitAnd = false
			p.l.mustLexNext()
			p.l.skipSpaces()
		}

		// This implements precedence order between `&&` and `||`. A `&&` is parsed with the smallest
		// next unit so it takes precedences while `||` parse with the longuest possibility.
		parser := p.parseUnaryExpression
		depthIncrease := 0
		if p.l.isOrOperator(next) {
			parser = p.parseExpression
			depthIncrease = 1
		}

		right, err := parser(depth + depthIncrease)

		switch {
		case isImplicitAnd || p.l.isAndOperator(next):
			if err != nil {
				if isImplicitAnd {
					return nil, fmt.Errorf("missing expression after implicit 'and' clause: %w", err)
				}

				return nil, fmt.Errorf("missing expression after 'and' clause: %w", err)
			}

			if v, ok := left.(*AndExpression); ok {
				v.Children = append(v.Children, right)
			} else {
				left = &AndExpression{Children: []Expression{left, right}}
			}

		case p.l.isOrOperator(next):
			if err != nil {
				return nil, fmt.Errorf("missing expression after 'or' clause: %w", err)
			}

			// It's impossible to coascle `||` expressions since they are recursive
			left = &OrExpression{Children: []Expression{left, right}}

		default:
			if err != nil {
				return nil, fmt.Errorf("unable to parse right hand side expression: %w", err)
			}

			return nil, parserError(fmt.Sprintf("token type %s is not valid binary right hand side expression", p.l.getTokenType(next)), next.Pos)
		}
	}
}

func (p *Parser) parseUnaryExpression(depth int) (Expression, error) {
	p.l.skipSpaces()

	token, err := p.l.Peek(0)
	if err != nil {
		return nil, err
	}

	if token.EOF() {
		return nil, parserError("expected a key term, minus sign or left parenthesis, got end of input", token.Pos)
	}

	switch {
	case p.l.isName(token) || p.l.isQuoting(token):
		return p.parseKeyTerm()
	case p.l.isLeftParenthesis(token):
		return p.parseParenthesisExpression(depth)
	case p.l.isNotOperator(token):
		return nil, fmt.Errorf("NOT operator (-) is not supported in the block filter")
	default:
		return nil, parserError(fmt.Sprintf("expected a key term, minus sign or left parenthesis, got %s", p.l.getTokenType(token)), token.Pos)
	}
}

func (p *Parser) parseParenthesisExpression(depth int) (Expression, error) {
	// Consume left parenthesis
	openingParenthesis := p.l.mustLexNext()
	p.lookForRightParenthesis++

	child, err := p.parseExpression(depth + 1)
	if err != nil {
		return nil, fmt.Errorf("invalid expression after opening parenthesis: %w", err)
	}

	p.l.skipSpaces()
	token, err := p.l.Next()
	if err != nil {
		return nil, err
	}

	if token.EOF() {
		return nil, parserError("expecting closing parenthesis, got end of input", openingParenthesis.Pos)
	}

	if !p.l.isRightParenthesis(token) {
		return nil, parserError(fmt.Sprintf("expecting closing parenthesis after expression, got %s", p.l.getTokenType(token)), token.Pos)
	}

	p.lookForRightParenthesis--
	return &ParenthesisExpression{child}, nil
}

func (p *Parser) parseKeyTerm() (Expression, error) {
	token := p.l.mustLexNext()

	var value *StringLiteral
	switch {
	case p.l.isName(token):
		value = &StringLiteral{
			Value: token.String(),
		}
	case p.l.isQuoting(token):
		literal, err := p.parseQuotedString(token)
		if err != nil {
			return nil, err
		}

		value = literal
	default:
		return nil, parserError(fmt.Sprintf("expecting key term, either a string or quoted string but got %s", p.l.getTokenType(token)), token.Pos)
	}

	return &KeyTerm{
		Value: value,
	}, nil
}

func (p *Parser) parseQuotedString(startQuoting lex.Token) (*StringLiteral, error) {
	builder := &strings.Builder{}
	for {
		token, err := p.l.Next()
		if err != nil {
			return nil, err
		}

		if token.EOF() {
			return nil, parserError(fmt.Sprintf("expecting closing quoting char %q, got end of input", startQuoting.Value), startQuoting.Pos)
		}

		if p.l.isQuoting(token) {
			value := builder.String()
			if value == "" {
				return nil, rangeParserError("an empty string is not valid", startQuoting.Pos, token.Pos)
			}

			return &StringLiteral{
				Value:       value,
				QuotingChar: startQuoting.Value,
			}, nil
		}

		builder.WriteString(token.Value)
	}
}
