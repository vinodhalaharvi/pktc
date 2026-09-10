// Package sexp reads S-expressions into a generic value tree.
//
// It knows nothing about packets. Turning these values into protocol
// nodes is the job of package ast.
package sexp

import "strings"

type Kind uint8

const (
	KList Kind = iota
	KSymbol
	KKeyword // :foo
	KString  // "foo"
	KNumber  // 100, 0x1001
	KStar    // *
)

func (k Kind) String() string {
	switch k {
	case KList:
		return "form"
	case KSymbol:
		return "symbol"
	case KKeyword:
		return "property name"
	case KString:
		return "string"
	case KNumber:
		return "number"
	case KStar:
		return "'*'"
	}
	return "unknown"
}

// Value is one node of the generic S-expression tree.
// Items is populated for KList; Text for every other kind.
type Value struct {
	Kind  Kind
	Text  string
	Items []Value
	Pos   Pos
}

// Key strips the leading colon from a KKeyword's text.
func (v Value) Key() string { return strings.TrimPrefix(v.Text, ":") }
