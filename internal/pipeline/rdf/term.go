// Package rdf provides the minimal RDF term, quad and N-Quads handling the
// harvesting pipeline needs, plus the IRI minter that gives derived structure
// stable, content-derived identity across runs.
//
// This is deliberately not a general-purpose RDF library: the pipeline only
// ever writes RDF, and only in N-Quads, so parsing and graph algebra are out
// of scope.
package rdf

import (
	"strings"
	"unicode/utf8"
)

// Kind distinguishes the three term positions N-Quads can hold.
type Kind int

const (
	KindIRI Kind = iota
	KindLiteral
	KindBlank
)

// Term is one RDF term. Datatype and Lang are only meaningful on literals and
// are mutually exclusive; a literal with neither is an implicit xsd:string.
type Term struct {
	Kind     Kind
	Value    string
	Datatype string // IRI, literals only
	Lang     string // BCP 47 tag, literals only
}

// IRI builds an IRI term.
func IRI(s string) Term { return Term{Kind: KindIRI, Value: s} }

// Literal builds a plain (implicitly xsd:string) literal.
func Literal(s string) Term { return Term{Kind: KindLiteral, Value: s} }

// TypedLiteral builds a literal with an explicit datatype IRI.
func TypedLiteral(s, datatype string) Term {
	return Term{Kind: KindLiteral, Value: s, Datatype: datatype}
}

// LangLiteral builds a language-tagged literal. An empty tag degrades to a
// plain literal rather than emitting invalid syntax, because source catalogues
// routinely carry empty language keys.
func LangLiteral(s, lang string) Term {
	if strings.TrimSpace(lang) == "" {
		return Literal(s)
	}
	return Term{Kind: KindLiteral, Value: s, Lang: lang}
}

// Blank builds a blank node term. The pipeline skolemizes rather than emitting
// blank nodes (R-CAT-3); this exists to round-trip source data that still
// carries them.
func Blank(id string) Term { return Term{Kind: KindBlank, Value: id} }

// IsZero reports whether the term is unset, so callers can skip optional
// fields without a parallel bool.
func (t Term) IsZero() bool { return t.Value == "" }

// String renders the term in N-Triples syntax.
func (t Term) String() string {
	switch t.Kind {
	case KindIRI:
		return "<" + escapeIRI(t.Value) + ">"
	case KindBlank:
		return "_:" + escapeBlankID(t.Value)
	default:
		var b strings.Builder
		b.WriteByte('"')
		b.WriteString(escapeLiteral(t.Value))
		b.WriteByte('"')
		switch {
		case t.Lang != "":
			b.WriteByte('@')
			b.WriteString(t.Lang)
		case t.Datatype != "":
			b.WriteString("^^<")
			b.WriteString(escapeIRI(t.Datatype))
			b.WriteByte('>')
		}
		return b.String()
	}
}

// escapeIRI makes an arbitrary string safe inside an N-Triples IRIREF.
// The characters excluded by the IRIREF production, plus anything below U+0020,
// are written as \uXXXX escapes, which the grammar permits.
func escapeIRI(s string) string {
	if !strings.ContainsAny(s, "<>\"{}|^`\\ \t\n\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case r < 0x20, r == '<', r == '>', r == '"', r == '{', r == '}',
			r == '|', r == '^', r == '`', r == '\\', r == ' ':
			writeUnicodeEscape(&b, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeLiteral escapes a string for an N-Triples STRING_LITERAL_QUOTE.
func escapeLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == utf8.RuneError {
				writeUnicodeEscape(&b, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeBlankID reduces a label to the BLANK_NODE_LABEL character set. Source
// blank node labels are arbitrary, so anything outside it becomes '_'.
func escapeBlankID(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case (r == '-' || r == '.' || r == '_') && i > 0:
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "b"
	}
	return b.String()
}

const hexDigits = "0123456789ABCDEF"

func writeUnicodeEscape(b *strings.Builder, r rune) {
	if r > 0xFFFF {
		b.WriteString(`\U`)
		for shift := 28; shift >= 0; shift -= 4 {
			b.WriteByte(hexDigits[(r>>shift)&0xF])
		}
		return
	}
	b.WriteString(`\u`)
	for shift := 12; shift >= 0; shift -= 4 {
		b.WriteByte(hexDigits[(r>>shift)&0xF])
	}
}
