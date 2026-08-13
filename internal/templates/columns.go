package templates

// columns.go lets a synchronously-rendered sparqlTable find its own
// <sparql-column> declarations.
//
// The async path never needs this: an /api fragment handler builds the params
// map itself and folds the declarations in before rendering. A sync table has no
// handler — its dict is written inline in the page template — so the partial has
// to look the declarations up while it renders.
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

// columnIconVar returns the SPARQL variable whose value carries the row icon for
// the given table, or "" when the table declares no icon column.
//
// Exposed to templates as {{ columnIconVar (templateSet) $id }}. It takes the
// set name explicitly because {{ templateSet }} is bound per set at load time
// (see Load) and is the key the index is scoped by.
func columnIconVar(set, id string) string {
	if columnLookup == nil || set == "" || id == "" {
		return ""
	}
	return columnLookup(set, id).IconVar()
}
