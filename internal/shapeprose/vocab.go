package shapeprose

// Namespaces. Hard-coded like bnodetree's: these terms steer the wording, there
// is nothing to configure per deployment.
const (
	sh       = "http://www.w3.org/ns/shacl#"
	rdf      = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	rdfs     = "http://www.w3.org/2000/01/rdf-schema#"
	owl      = "http://www.w3.org/2002/07/owl#"
	xsd      = "http://www.w3.org/2001/XMLSchema#"
	schema   = "http://schema.org/"
	schemaS  = "https://schema.org/"
	qudt     = "http://qudt.org/schema/qudt/"
	cube     = "https://cube.link/"
	meta     = "https://cube.link/meta/"
	relation = "https://cube.link/relation/"
	timeNS   = "http://www.w3.org/2006/time#"
	geo      = "http://www.opengis.net/ont/geosparql#"
	dct      = "http://purl.org/dc/terms/"
	prov     = "http://www.w3.org/ns/prov#"

	rdfType  = rdf + "type"
	rdfFirst = rdf + "first"
	rdfRest  = rdf + "rest"
	rdfNil   = rdf + "nil"
)

// cubeCreatorNS covers the Cube Creator's bookkeeping vocabularies
// (cube-creator.zazuko.com/vocab#, …/shared-dimensions/vocab#): editor state,
// nothing a reader of the shape needs.
const cubeCreatorNS = "https://cube-creator.zazuko.com/"

// datatypeCodes name common datatypes in plain words (shape.datatype.<code>).
// Anything else is shown as its CURIE.
var datatypeCodes = map[string]string{
	xsd + "string":             "text",
	xsd + "normalizedString":   "text",
	xsd + "token":              "text",
	rdf + "langString":         "langText",
	rdf + "HTML":               "html",
	xsd + "integer":            "integer",
	xsd + "int":                "integer",
	xsd + "long":               "integer",
	xsd + "short":              "integer",
	xsd + "nonNegativeInteger": "integer",
	xsd + "positiveInteger":    "integer",
	xsd + "decimal":            "number",
	xsd + "double":             "number",
	xsd + "float":              "number",
	xsd + "boolean":            "boolean",
	xsd + "date":               "date",
	xsd + "dateTime":           "dateTime",
	xsd + "dateTimeStamp":      "dateTime",
	xsd + "time":               "time",
	xsd + "gYear":              "year",
	xsd + "gYearMonth":         "yearMonth",
	xsd + "duration":           "duration",
	xsd + "anyURI":             "uri",
	geo + "wktLiteral":         "geometry",
	cube + "Undefined":         "undefined",
}

// nodeKindCodes for sh:nodeKind (shape.nodeKind.<code>).
var nodeKindCodes = map[string]string{
	sh + "IRI":                "iri",
	sh + "BlankNode":          "blank",
	sh + "Literal":            "literal",
	sh + "BlankNodeOrIRI":     "blankOrIri",
	sh + "BlankNodeOrLiteral": "blankOrLiteral",
	sh + "IRIOrLiteral":       "iriOrLiteral",
}

// dimensionCodes for the cube.link dimension types (shape.dimension.<code>).
var dimensionCodes = map[string]string{
	cube + "KeyDimension":       "key",
	cube + "MeasureDimension":   "measure",
	cube + "AttributeDimension": "attribute",
}

// scaleCodes for qudt:scaleType (shape.scale.<code>).
var scaleCodes = map[string]string{
	qudt + "NominalScale":  "nominal",
	qudt + "OrdinalScale":  "ordinal",
	qudt + "IntervalScale": "interval",
	qudt + "RatioScale":    "ratio",
}

// dataKindCodes for the type of a meta:dataKind node (shape.dataKind.<code>).
var dataKindCodes = map[string]string{
	timeNS + "GeneralDateTimeDescription": "time",
	schema + "GeoShape":                   "geoShape",
	schema + "GeoCoordinates":             "geoCoordinates",
	schemaS + "GeoShape":                  "geoShape",
	schemaS + "GeoCoordinates":            "geoCoordinates",
}

// timeUnitCodes for time:unitType. Prefixed, so every code is unique across
// families and the template maps them in one place (shape.code.<code>).
var timeUnitCodes = map[string]string{
	timeNS + "unitYear":   "unitYear",
	timeNS + "unitMonth":  "unitMonth",
	timeNS + "unitWeek":   "unitWeek",
	timeNS + "unitDay":    "unitDay",
	timeNS + "unitHour":   "unitHour",
	timeNS + "unitMinute": "unitMinute",
	timeNS + "unitSecond": "unitSecond",
}

// relationCodes for the type of a meta:dimensionRelation node.
var relationCodes = map[string]string{
	relation + "StandardError": "standardError",
}

// Metadata predicates: read for the label, description, tooltip, badge and
// sort order, never explained as constraints.
var (
	namePreds = []string{sh + "name", schema + "name", schemaS + "name", rdfs + "label", dct + "title"}
	descPreds = []string{sh + "description", schema + "description", schemaS + "description", rdfs + "comment", dct + "description", sh + "intent"}
)

// ignored predicates carry nothing a reader needs: type declarations of the
// shape itself and editor bookkeeping. rdf:type is handled separately (cube
// dimension types are explained, the rest ignored).
var ignored = map[string]bool{
	sh + "message":                      true, // tooltip
	sh + "severity":                     true, // badge
	sh + "order":                        true, // sort
	sh + "group":                        true, // grouping
	sh + "deactivated":                  true, // flag
	sh + "path":                         true, // the line itself
	sh + "property":                     true, // the lines
	schema + "version":                  true,
	schemaS + "version":                 true,
	prov + "wasDerivedFrom":             true,
	sh + "targetClass":                  true, // shape level, handled there
	sh + "targetNode":                   true,
	sh + "targetSubjectsOf":             true,
	sh + "targetObjectsOf":              true,
	sh + "closed":                       true,
	sh + "ignoredProperties":            true,
	sh + "qualifiedMinCount":            true, // read with sh:qualifiedValueShape
	sh + "qualifiedMaxCount":            true,
	sh + "qualifiedValueShapesDisjoint": true,
	sh + "flags":                        true, // read with sh:pattern
}
