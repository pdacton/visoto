# A full `<IRI>` inline in a template query is silently stripped

**Status:** Open
**Component:** template scanning (`internal/parser/template.go`), `<sparql-async>` / `<sparql-query>` / `<sparql-facet>` declarations
**Workaround in place:** always use a prefixed name; add the prefix to `visoto.config`

## Problem

Query declarations live in the template as element *text*, and the scanner reads
them with `html.Parse` (`internal/parser/template.go`). An absolute IRI written in
SPARQL's angle-bracket form is therefore indistinguishable from markup: the HTML
parser treats `<http://www.opengis.net/ont/geosparql#hasGeometry>` as a start tag
and drops it from the element's text content.

The predicate simply disappears from the query. Nothing warns, at scan time or at
request time.

## Observed symptom

In `templates/classes/https%3A%2F%2Fcube-creator.zazuko.com%2Fshared-dimensions%2Fvocab%23SharedDimensionTerm.html`,
this line:

```sparql
OPTIONAL { ?instance <http://www.opengis.net/ont/geosparql#hasGeometry> ?geometry_ . }
```

reached the endpoint as:

```sparql
OPTIONAL { ?instance  ?geometry_ . }
```

LINDAS answered `400 MALFORMED QUERY: Encountered " "." ". "" at line 18, column 46`.
In the UI the table just stays empty — no error, no partial render, no console
message. The server log shows only `SPARQL endpoint returned HTTP error
status_code=400`, because the error path discards the response body
(`internal/sparql/query.go`), so neither the failing query nor the endpoint's
explanation is visible.

This is expensive to diagnose. The same query pasted into the MCP tool or the
SPARQL console works perfectly, which points suspicion at everything except the
template: the working-set rewriting, `GROUP BY` interactions with the count query,
the `??` substitution. All were investigated and cleared before the actual cause
turned up — and only by temporarily patching `query.go` to log the query text.

## Current workaround

Use a prefixed name and declare the prefix in `visoto.config`:

```toml
"PREFIX geo:     <http://www.opengis.net/ont/geosparql#>",
```

```sparql
OPTIONAL { ?instance geo:hasGeometry ?geometry_ . }
```

Every other template already happens to do this, which is why the bug went
unnoticed until a vocabulary without a configured prefix was needed.

## Proposed fix

Handle it in the substitution/scan layer rather than relying on authors knowing
the rule.

Preferred: extract the raw text of query-declaration elements without letting the
HTML parser see the body — e.g. locate `<sparql-async …>…</sparql-async>` spans
directly and take the literal inner text, so angle-bracket IRIs survive. The
declarations are simple, non-nested elements, so this does not need the full HTML
parser.

Cheaper alternative: keep `html.Parse`, but pre-escape `<…>` sequences that look
like IRIs (`<` + scheme + `://` … + `>`) to `&lt;…&gt;` before parsing, and unescape
after extraction. Narrower, but it leaves the general "text goes through an HTML
parser" hazard in place.

Fallback if neither is done: detect the damage and refuse to run. A declared query
that contains a doubled space where a predicate should be, or that lost text
between scan input and scan output, could raise a startup error naming the
template — startup already fails on other authoring mistakes (undefined i18n keys,
unresolvable `{{ template }}` includes), so this would fit the existing model.

Independently worth doing: include the endpoint's response body (and the query) in
the `SPARQL endpoint returned HTTP error` log line in `internal/sparql/query.go`.
The body is currently copied to `io.Discard`. Having it would have shortened this
particular hunt from a long detour to a single log read, and would help with any
other malformed-query bug.

## Notes

- Also applies to `<sparql-query>` and `<sparql-facet>` declarations — same scanner,
  same text extraction.
- Not related to the `html/template` JS/CSS escaping traps (`ZgotmplZ`, `toJSON`
  double-quoting): those happen at render time in Go's template engine, whereas
  this happens earlier, when the scanner parses the template file as HTML.
- A Go template comment is not an HTML comment, so commenting out a declaration
  containing an inline IRI does not protect it from the scanner either.
