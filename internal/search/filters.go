package search

// GetClassFilters returns hardcoded list of common RDF classes
// These are intentionally hardcoded for performance and simplicity
func GetClassFilters() []Filter {
	return []Filter{
		{Label: "Any Class", IRI: ""},
		{Label: "Organization (schema)", IRI: "http://schema.org/Organization"},
		{Label: "Person (schema)", IRI: "http://schema.org/Person"},
		{Label: "Zefix Organisation", IRI: "https://schema.ld.admin.ch/ZefixOrganisation"},
		{Label: "Defined Term", IRI: "http://schema.org/DefinedTerm"},
		{Label: "OWL Class", IRI: "http://www.w3.org/2002/07/owl#Class"},
		{Label: "RDFS Class", IRI: "http://www.w3.org/2000/01/rdf-schema#Class"},
	}
}

// GetPropertyFilters returns hardcoded list of common RDF properties
func GetPropertyFilters() []Filter {
	return []Filter{
		{Label: "Any Property", IRI: ""},
		{Label: "rdfs:label", IRI: "http://www.w3.org/2000/01/rdf-schema#label"},
		{Label: "schema:name", IRI: "http://schema.org/name"},
		{Label: "schema:description", IRI: "http://schema.org/description"},
		{Label: "skos:prefLabel", IRI: "http://www.w3.org/2004/02/skos/core#prefLabel"},
		{Label: "skos:altLabel", IRI: "http://www.w3.org/2004/02/skos/core#altLabel"},
	}
}
