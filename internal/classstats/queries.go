package classstats

// Queries sent by the collector and the class-tree route. Measured on LINDAS
// prod (GraphDB, ~470 M triples), 2026-09-27.

// statisticsGraph is GraphDB's pseudo-graph: a query over it with ONE triple
// pattern whose predicate and object are bound is answered from index
// statistics instead of a scan — `?s a <C>` for a 14 M-instance class in
// 0.2 s where a live COUNT takes 16 s. The numbers are approximate. Batching
// classes with VALUES (or a GROUP BY) defeats the fast path and falls back to
// a live count (19 s for three classes), so the collector asks one class at
// a time. Other stores: QLever answers 0 (an empty named graph), Virtuoso 400.
const statisticsGraph = "http://www.ontotext.com/owlim/system#statistics"

// queryStatsTriples is the whole store's size from the statistics (0.16 s).
// A result > 0 is how the collector recognises GraphDB.
const queryStatsTriples = `SELECT (COUNT(*) AS ?n) FROM <` + statisticsGraph + `> WHERE { ?s ?p ?o }`

// queryTriples is the live store size: instant on QLever, a timeout on large
// GraphDB stores (which never get here).
const queryTriples = `SELECT (COUNT(*) AS ?n) WHERE { ?s ?p ?o }`

// queryStatsClass / queryClass count one class; %s is an IRI term (<…>).
const queryStatsClass = `SELECT (COUNT(*) AS ?n) FROM <` + statisticsGraph + `> WHERE { ?s a %s }`
const queryClass = `SELECT (COUNT(*) AS ?n) WHERE { ?s a %s }`

// ClassTreeQuery lists the declared classes that have instances — directly or
// through a subclass, so the hierarchy stays connected — with their labels (all
// languages) and parents: 589 classes in 0.36 s on LINDAS, where "every class in
// use" (SELECT DISTINCT ?class { ?s a ?class }) times out. Classes used without
// being declared rdfs:Class / owl:Class are not found; that is the price.
//
// The variable names are Graph Explorer's classTreeQuery bindings, so the
// /api/class-tree route can hand the result to GE's own tree builder.
const ClassTreeQuery = `PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX owl: <http://www.w3.org/2002/07/owl#>
SELECT ?class ?label ?parent WHERE {
  { ?class a rdfs:Class } UNION { ?class a owl:Class }
  FILTER ISIRI(?class)
  FILTER EXISTS { ?sub rdfs:subClassOf* ?class . ?i a ?sub }
  OPTIONAL { ?class rdfs:label ?label }
  OPTIONAL { ?class rdfs:subClassOf ?parent . FILTER ISIRI(?parent) }
}`
