// Package highlight_go provides a syntax highlighter for Go, using go/scanner.
package highlight_go

import (
	"io"

	"github.com/shurcooL/highlight_go/internal/go/scanner"
	"github.com/shurcooL/highlight_go/internal/go/token"

	"github.com/sourcegraph/annotate"
	"github.com/sourcegraph/syntaxhighlight"
)

// TODO: Stop using internal copies of go/scanner and go/token in Go 1.12.

// TokenKind returns a syntaxhighlight token kind value for the given tok and lit.
func TokenKind(tok token.Token, lit string) syntaxhighlight.Kind {
	switch {
	case tok.IsKeyword() || (tok.IsOperator() && tok <= token.ELLIPSIS):
		return syntaxhighlight.Keyword

	// Literals.
	case tok == token.INT || tok == token.FLOAT || tok == token.IMAG || tok == token.CHAR:
		return syntaxhighlight.Decimal
	case tok == token.STRING:
		return syntaxhighlight.String
	case lit == "true" || lit == "false" || lit == "iota" || lit == "nil":
		return syntaxhighlight.Literal

	case tok == token.COMMENT:
		return syntaxhighlight.Comment
	default:
		return syntaxhighlight.Plaintext
	}
}

func Print(src []byte, w io.Writer, p syntaxhighlight.Printer) error {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	s.Init(file, src, nil, scanner.ScanComments)

	prevEndOffset := 0

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}

		offset := fset.Position(pos).Offset

		// Print whitespace between previous token and current token, if any.
		if prevEndOffset < offset {
			err := p.Print(w, syntaxhighlight.Whitespace, string(src[prevEndOffset:offset]))
			if err != nil {
				return err
			}
		}

		text := tokenText(tok, lit)

		// Print token.
		err := p.Print(w, TokenKind(tok, lit), text)
		if err != nil {
			return err
		}

		prevEndOffset = offset + len(text)
	}

	// Print final whitespace between last token and EOF, if any.
	if prevEndOffset < len(src) {
		err := p.Print(w, syntaxhighlight.Whitespace, string(src[prevEndOffset:]))
		if err != nil {
			return err
		}
	}

	return nil
}

func Annotate(src []byte, a syntaxhighlight.Annotator) (annotate.Annotations, error) {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	s.Init(file, src, nil, scanner.ScanComments)

	var anns annotate.Annotations

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}

		// Annotate token.
		ann, err := a.Annotate(fset.Position(pos).Offset, TokenKind(tok, lit), tokenText(tok, lit))
		if err != nil {
			return nil, err
		}
		if ann == nil {
			continue
		}
		anns = append(anns, ann)
	}

	return anns, nil
}

func tokenText(tok token.Token, lit string) string {
	if lit == "" {
		return tok.String()
	}
	return lit
}
