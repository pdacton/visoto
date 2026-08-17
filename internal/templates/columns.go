package templates

// columns.go lets a synchronously-rendered sparqlTable find its own
// <sparql-column> declarations.
//
// The async path never needs this: an /api fragment handler builds the params
// map itself and folds the declarations in before rendering. A sync table has no
// handler — its dict is written inline in the page template — so the partial has
// to look the declarations up while it renders. Every rendering role a column can
// carry (icon, badge) needs one of these, or that role would be declarable only on
// async tables.
//
// The index those declarations live in is built in package main at startup, from
// the same template sets. Rather than move the whole indexer down here (or import
// upward, which is impossible), main registers a lookup function.

import "hutzli.org/visoto/internal/column"

// columnLookup answers "which columns are declared for this base query id, in
// this template set". Nil until main registers the real one, in which case every
// sync table simply finds no declarations — the same as declaring none.
var columnLookup func(set, id string) column.Table

// SetColumnLookup installs the column index for sync table rendering. Call once
// at startup, after the index is built and before any request is served.
func SetColumnLookup(fn func(set, id string) column.Table) {
	columnLookup = fn
}

// columnIconVars returns the SPARQL variables whose values carry a row icon for
// the given table, comma-separated, or "" when the table declares no icon column.
//
// Exposed to templates as {{ columnIconVars (templateSet) $id }}. It takes the
// set name explicitly because {{ templateSet }} is bound per set at load time
// (see Load) and is the key the index is scoped by.
func columnIconVars(set, id string) string {
	return lookupVars(set, id, column.Table.IconVars)
}

// columnBadgeVars is the same lookup for <sparql-column … badge>, exposed as
// {{ columnBadgeVars (templateSet) $id }}.
func columnBadgeVars(set, id string) string {
	return lookupVars(set, id, column.Table.BadgeVars)
}

// lookupVars resolves a table's declarations and projects one role off them. The
// nil-lookup and empty-key guards live here so every role answers "no columns
// declared" identically rather than each restating the rule.
func lookupVars(set, id string, role func(column.Table) string) string {
	if columnLookup == nil || set == "" || id == "" {
		return ""
	}
	return role(columnLookup(set, id))
}
