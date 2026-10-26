package sparql

import (
	"regexp"
	"strings"
)

// concatSeparatorRe matches an aggregate "GROUP_CONCAT(<expr>; SEPARATOR="…") AS ?var".
// The expression is anything up to the first ";" — inside GROUP_CONCAT a ";" can
// only introduce the separator — so a GROUP_CONCAT without one never borrows the
// separator of a later aggregate. Groups: 1 = double-quoted, 2 = single-quoted
// separator, 3 = the alias.
var concatSeparatorRe = regexp.MustCompile(
	`(?is)GROUP_CONCAT\s*\([^;]*;\s*SEPARATOR\s*=\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')\s*\)\s*AS\s+[?$](\w+)`)

var literalUnescaper = strings.NewReplacer(`\t`, "\t", `\n`, "\n", `\r`, "\r", `\"`, `"`, `\'`, `'`, `\\`, `\`)

// ConcatSeparators returns, per projected variable, the separator its
// GROUP_CONCAT joins values with — the columns the table frontend may split
// back into individual values (to group by each one).
//
// Only an explicit, non-empty SEPARATOR qualifies. The default separator is a
// single space, and splitting on it would cut every multi-word label apart; the
// values themselves are never inspected for the same reason.
func ConcatSeparators(query string) map[string]string {
	out := map[string]string{}
	for _, m := range concatSeparatorRe.FindAllStringSubmatch(query, -1) {
		sep := m[1]
		if sep == "" {
			sep = m[2]
		}
		sep = literalUnescaper.Replace(sep)
		if sep != "" {
			out[m[3]] = sep
		}
	}
	return out
}
