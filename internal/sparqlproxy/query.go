package sparqlproxy

import "strings"

// readOnlyKeywords are the four SPARQL query forms. Everything else in the
// grammar's top level — INSERT, DELETE, LOAD, CLEAR, DROP, CREATE, ADD, MOVE,
// COPY, WITH — is an Update operation and is refused.
//
// This is a positive allowlist on purpose. A blocklist would have to reason
// about "DELETE" appearing inside a string literal or an IRI, which is exactly
// the ambiguity that makes such checks fail open.
var readOnlyKeywords = map[string]bool{
	"SELECT":    true,
	"ASK":       true,
	"CONSTRUCT": true,
	"DESCRIBE":  true,
}

// IsReadOnlyQuery reports whether q is a SPARQL *query* (read) rather than an
// *update* (write).
//
// It strips comments and any leading BASE/PREFIX declarations, then requires
// the first remaining keyword to be one of readOnlyKeywords.
//
// It fails CLOSED: a query whose leading keyword cannot be identified — empty,
// comments only, or anything unrecognised — is refused. The proxy attaches the
// endpoint's credentials only after this returns true, so a false positive here
// would hand a browser a write-enabled token.
func IsReadOnlyQuery(q string) bool {
	kw, ok := leadingKeyword(q)
	return ok && readOnlyKeywords[kw]
}

// leadingKeyword returns the first significant keyword of q, uppercased,
// skipping comments and BASE/PREFIX declarations. ok is false when there is no
// such keyword.
func leadingKeyword(q string) (string, bool) {
	s := stripComments(q)
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", false
		}
		word, rest := nextWord(s)
		if word == "" {
			return "", false
		}
		switch strings.ToUpper(word) {
		case "BASE":
			// BASE <iri> — skip the declaration and keep looking.
			_, rest = nextIRI(rest)
			s = rest
		case "PREFIX":
			// PREFIX name: <iri> — skip both tokens.
			_, rest = nextWord(strings.TrimSpace(rest))
			_, rest = nextIRI(rest)
			s = rest
		default:
			return strings.ToUpper(word), true
		}
	}
}

// nextWord splits off the leading run of non-space characters.
func nextWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

// nextIRI skips a <...> token. If s does not start with one, it is returned
// unchanged so the caller still makes progress.
func nextIRI(s string) (string, string) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "<") {
		return "", s
	}
	if end := strings.IndexByte(s, '>'); end >= 0 {
		return s[:end+1], s[end+1:]
	}
	return s, ""
}

// stripComments removes SPARQL comments: '#' to end of line.
//
// The whole subtlety of this function is that '#' is only a comment when it is
// NOT inside an IRI or a string literal. Both are extremely common in real
// queries:
//
//	PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
//
// A naive strip eats that '>' and everything after it, swallowing the entire
// query and making a legitimate SELECT unrecognisable. So the scanner tracks
// whether it sits inside <>, "...", '...', """...""" or ”'...”'.
func stripComments(q string) string {
	var b strings.Builder
	b.Grow(len(q))

	const (
		normal = iota
		inIRI
		inString
	)
	state := normal
	var quote byte   // the quote char that opened the current string
	var quoteLen int // 1 or 3, so a long literal is closed by the matching run

	for i := 0; i < len(q); i++ {
		c := q[i]
		switch state {
		case normal:
			switch {
			case c == '#':
				// Comment: skip to end of line, keeping the newline so token
				// boundaries survive.
				for i < len(q) && q[i] != '\n' {
					i++
				}
				if i < len(q) {
					b.WriteByte('\n')
				}
				continue
			case c == '<':
				// Only an IRI if it is not a comparison operator. Inside a
				// leading declaration it always is, and a stray '<' elsewhere
				// costs us nothing but a slightly longer scan.
				state = inIRI
			case c == '"' || c == '\'':
				state, quote = inString, c
				quoteLen = 1
				if i+2 < len(q) && q[i+1] == c && q[i+2] == c {
					quoteLen = 3
					b.WriteByte(c)
					b.WriteByte(c)
					i += 2
				}
			}
		case inIRI:
			if c == '>' {
				state = normal
			}
		case inString:
			if c == '\\' && i+1 < len(q) {
				// Escaped char: copy both, so \" does not close the literal.
				b.WriteByte(c)
				i++
				c = q[i]
				break
			}
			if c == quote {
				if quoteLen == 1 {
					state = normal
				} else if i+2 < len(q) && q[i+1] == c && q[i+2] == c {
					b.WriteByte(c)
					b.WriteByte(c)
					i += 2
					state = normal
				}
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}
