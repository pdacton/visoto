package rdf

// Namespaces used by the pipeline. Viso is the pipeline's own term namespace;
// everything else is reused unchanged (§6.1 of the pipeline plan).
const (
	NSViso   = "https://visoto.hutzli.org/ns/structure#"
	NSRDF    = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NSRDFS   = "http://www.w3.org/2000/01/rdf-schema#"
	NSDCAT   = "http://www.w3.org/ns/dcat#"
	NSDCT    = "http://purl.org/dc/terms/"
	NSFOAF   = "http://xmlns.com/foaf/0.1/"
	NSPROV   = "http://www.w3.org/ns/prov#"
	NSSHACL  = "http://www.w3.org/ns/shacl#"
	NSCSVW   = "http://www.w3.org/ns/csvw#"
	NSVOID   = "http://rdfs.org/ns/void#"
	NSXSD    = "http://www.w3.org/2001/XMLSchema#"
	NSSKOS   = "http://www.w3.org/2004/02/skos/core#"
	NSVCARD  = "http://www.w3.org/2006/vcard/ns#"
	NSSCHEMA = "http://schema.org/"
)

// Viso builds a term IRI in the pipeline namespace.
func Viso(local string) Term { return IRI(NSViso + local) }

// Dcat builds a term IRI in the DCAT namespace.
func Dcat(local string) Term { return IRI(NSDCAT + local) }

// Dct builds a term IRI in the Dublin Core terms namespace.
func Dct(local string) Term { return IRI(NSDCT + local) }

// Foaf builds a term IRI in the FOAF namespace.
func Foaf(local string) Term { return IRI(NSFOAF + local) }

// Prov builds a term IRI in the PROV-O namespace.
func Prov(local string) Term { return IRI(NSPROV + local) }

// Xsd builds a datatype IRI in the XML Schema namespace.
func Xsd(local string) string { return NSXSD + local }

// Frequently used terms, named once so a typo is a compile error.
var (
	A = IRI(NSRDF + "type")

	RDFValue  = IRI(NSRDF + "value")
	RDFSLabel = IRI(NSRDFS + "label")

	DcatDataset      = Dcat("Dataset")
	DcatDistribution = Dcat("Distribution")
	DcatCatalog      = Dcat("Catalog")
	DcatHasDist      = Dcat("distribution")
	DcatDownloadURL  = Dcat("downloadURL")
	DcatAccessURL    = Dcat("accessURL")
	DcatMediaType    = Dcat("mediaType")
	DcatByteSize     = Dcat("byteSize")
	DcatKeyword      = Dcat("keyword")
	DcatTheme        = Dcat("theme")
	DcatContactPoint = Dcat("contactPoint")
	DcatLandingPage  = Dcat("landingPage")

	DctTitle              = Dct("title")
	DctDescription        = Dct("description")
	DctIdentifier         = Dct("identifier")
	DctPublisher          = Dct("publisher")
	DctModified           = Dct("modified")
	DctIssued             = Dct("issued")
	DctLicense            = Dct("license")
	DctFormat             = Dct("format")
	DctRights             = Dct("rights")
	DctAccrualPeriodicity = Dct("accrualPeriodicity")

	FoafAgent = Foaf("Agent")
	FoafName  = Foaf("name")

	ProvActivity       = Prov("Activity")
	ProvWasGeneratedBy = Prov("wasGeneratedBy")
	ProvStartedAtTime  = Prov("startedAtTime")
	ProvEndedAtTime    = Prov("endedAtTime")

	// Pipeline vocabulary.
	VisoHarvestRun          = Viso("HarvestRun")
	VisoCatalogSource       = Viso("CatalogSource")
	VisoLastRun             = Viso("lastRun")
	VisoRunID               = Viso("runID")
	VisoSource              = Viso("source")
	VisoSourceType          = Viso("sourceType")
	VisoRunStatus           = Viso("runStatus")
	VisoDatasetsSeen        = Viso("datasetsSeen")
	VisoDistributionsSeen   = Viso("distributionsSeen")
	VisoQuadsWritten        = Viso("quadsWritten")
	VisoCatalogGraph        = Viso("catalogGraph")
	VisoCurrentCatalogGraph = Viso("currentCatalogGraph")
	VisoHarvestedFrom       = Viso("harvestedFrom")
	VisoWatermark           = Viso("watermark")
	VisoDeclaredMediaType   = Viso("declaredMediaType")
)
