package upload

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pdacton/visoto/internal/config"
	"github.com/pdacton/visoto/internal/sparql"
)

// Vocabulary terms the ontology declaration reads and writes.
const (
	owlOntology = "http://www.w3.org/2002/07/owl#Ontology"
	rdfsComment = "http://www.w3.org/2000/01/rdf-schema#comment"
	dctermsSrc  = "http://purl.org/dc/terms/source"
)

var (
	rdfTypeTerm     = sparql.MustIRI("http://www.w3.org/1999/02/22-rdf-syntax-ns#type").Term()
	owlOntologyTerm = sparql.MustIRI(owlOntology).Term()
	rdfsCommentTerm = sparql.MustIRI(rdfsComment).Term()
	dctermsSrcTerm  = sparql.MustIRI(dctermsSrc).Term()
)

// generatedComment marks the triples as visoto's, not the source's, so they can
// always be told apart from the uploaded data.
const generatedComment = "Declared by visoto at upload: the source data has no owl:Ontology."

// termKinds are the declarations whose namespace identifies a vocabulary: the
// same kinds the owl:Ontology page counts as an ontology's terms.
var termKinds = []string{
	"http://www.w3.org/2002/07/owl#Class",
	"http://www.w3.org/2000/01/rdf-schema#Class",
	"http://www.w3.org/2002/07/owl#ObjectProperty",
	"http://www.w3.org/2002/07/owl#DatatypeProperty",
	"http://www.w3.org/2002/07/owl#AnnotationProperty",
	"http://www.w3.org/1999/02/22-rdf-syntax-ns#Property",
}

// OntologyResult reports what the "declare as ontology" option did. Exactly one
// of Declared / Existing is true when no error occurred.
type OntologyResult struct {
	IRI      string `json:"iri"`
	Declared bool   `json:"declared"` // visoto inserted the owl:Ontology triples
	Existing bool   `json:"existing"` // the graph already had an owl:Ontology; nothing inserted
}

// ontologyDeclaration types a freshly uploaded graph as an owl:Ontology when its
// data does not already declare one.
type ontologyDeclaration struct {
	ep     *config.SparqlEndpoint
	graph  sparql.IRI
	iri    string // requested ontology IRI; empty = derive from the data
	label  string
	source string // fetched URL, recorded as dct:source; empty in file mode
}

// declareOntology validates the form input and runs the declaration.
func declareOntology(ep *config.SparqlEndpoint, graphURI, ontologyIRI, label, source string) (OntologyResult, error) {
	if graphURI == "" {
		return OntologyResult{}, fmt.Errorf("a named graph is required")
	}
	graph, err := sparql.ParseIRI(graphURI)
	if err != nil {
		return OntologyResult{}, fmt.Errorf("graph URI %q cannot be used in SPARQL", graphURI)
	}
	return ontologyDeclaration{ep: ep, graph: graph, iri: ontologyIRI, label: label, source: source}.run()
}

// run checks for an existing owl:Ontology, resolves the ontology IRI and inserts
// the declaration.
func (d ontologyDeclaration) run() (OntologyResult, error) {
	existing, err := d.existingOntology()
	if err != nil {
		return OntologyResult{}, err
	}
	if existing != "" {
		return OntologyResult{IRI: existing, Existing: true}, nil
	}
	iriText := d.iri
	if iriText == "" {
		if iriText, err = d.derivedNamespace(); err != nil {
			return OntologyResult{}, err
		}
	}
	ontology, err := sparql.ParseIRI(iriText)
	if err != nil {
		return OntologyResult{}, fmt.Errorf("ontology IRI %q is not a valid IRI", iriText)
	}
	if err := sparqlUpdate(d.ep, d.insert(ontology)); err != nil {
		return OntologyResult{}, err
	}
	return OntologyResult{IRI: ontology.String(), Declared: true}, nil
}

// existingOntology returns an owl:Ontology the graph already declares, or "".
func (d ontologyDeclaration) existingOntology() (string, error) {
	q := fmt.Sprintf("SELECT ?o WHERE { GRAPH %s { ?o %s %s } } LIMIT 1",
		d.graph.Term(), rdfTypeTerm, owlOntologyTerm)
	rows, err := sparqlQuery(d.ep, q)
	if err != nil {
		return "", fmt.Errorf("checking for an existing owl:Ontology: %w", sparql.RedactTransportError(err))
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0]["o"].Value, nil
}

// derivedNamespace guesses the ontology IRI as the most common namespace among
// the classes and properties the graph declares.
func (d ontologyDeclaration) derivedNamespace() (string, error) {
	kinds := make([]string, len(termKinds))
	for i, k := range termKinds {
		kinds[i] = sparql.MustIRI(k).Term()
	}
	q := fmt.Sprintf("SELECT DISTINCT ?term WHERE { VALUES ?kind { %s } GRAPH %s { ?term %s ?kind } FILTER(isIRI(?term)) }",
		strings.Join(kinds, " "), d.graph.Term(), rdfTypeTerm)
	rows, err := sparqlQuery(d.ep, q)
	if err != nil {
		return "", fmt.Errorf("deriving the ontology IRI: %w", sparql.RedactTransportError(err))
	}
	terms := make([]string, 0, len(rows))
	for _, r := range rows {
		terms = append(terms, r["term"].Value)
	}
	ns := dominantNamespace(terms)
	if ns == "" {
		return "", fmt.Errorf("no classes or properties declared in the data to derive an ontology IRI from; enter one")
	}
	return ns, nil
}

// insert builds the INSERT DATA that declares the ontology inside the graph.
func (d ontologyDeclaration) insert(ontology sparql.IRI) string {
	o := ontology.Term()
	triples := fmt.Sprintf("%s %s %s . %s %s %s .",
		o, rdfTypeTerm, owlOntologyTerm, o, rdfsCommentTerm, sparqlLiteral(generatedComment))
	if d.label != "" {
		triples += fmt.Sprintf(" %s %s %s .", o, rdfsLabelTerm, sparqlLiteral(d.label))
	}
	if src, err := sparql.ParseIRI(d.source); d.source != "" && err == nil {
		triples += fmt.Sprintf(" %s %s %s .", o, dctermsSrcTerm, src.Term())
	}
	return fmt.Sprintf("INSERT DATA { GRAPH %s { %s } }", d.graph.Term(), triples)
}

// namespaceOf cuts an IRI after its last '#' or '/': the namespace a CURIE
// prefix would cover. "" when the IRI has neither past its scheme.
func namespaceOf(iri string) string {
	i := strings.LastIndexAny(iri, "#/")
	if i < 0 || i < strings.Index(iri, "://")+3 {
		return ""
	}
	return iri[:i+1]
}

// dominantNamespace returns the namespace shared by most terms; ties go to the
// lexically first, so the result is deterministic.
func dominantNamespace(terms []string) string {
	counts := map[string]int{}
	for _, t := range terms {
		if ns := namespaceOf(t); ns != "" {
			counts[ns]++
		}
	}
	namespaces := make([]string, 0, len(counts))
	for ns := range counts {
		namespaces = append(namespaces, ns)
	}
	sort.Slice(namespaces, func(i, j int) bool {
		if counts[namespaces[i]] != counts[namespaces[j]] {
			return counts[namespaces[i]] > counts[namespaces[j]]
		}
		return namespaces[i] < namespaces[j]
	})
	if len(namespaces) == 0 {
		return ""
	}
	return namespaces[0]
}
