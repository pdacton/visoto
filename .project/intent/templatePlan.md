# Template Coverage Plan — LINDAS prod

All **677** classes in LINDAS prod (`https://ld.admin.ch/query/`), by instance count,
checked against `templates/classes/` and `templates/instances/`.

- **Class / Instance** — ✅ a template file exists for that IRI, `—` none.
- **Needed** — `no` when the class has fewer than 50 instances, otherwise `yes`.
- Counts are a snapshot (2026-08-26); LINDAS counts drift between queries.

## Summary

| | Count |
|---|---|
| Classes in LINDAS prod | 677 |
| Classes needing a template (>= 50 instances) | 197 |
| ...of those, missing a **class** template | **167** |
| ...of those, missing an **instance** template | **171** |
| Classes below threshold (no template needed) | 480 |

Two class templates exist for classes with **no instances** in prod: `dcat:Catalog` and `euthority:MappedCode` (the closest live classes are `schema:DataCatalog`, 12 instances, and `vaem:CatalogEntry`, 5).

## All classes

| # | Class | Instances | Class tpl | Instance tpl | Needed |
|---:|---|---:|:---:|:---:|:---:|
| 1 | `cube:Observation` | 13,227,138 | ✅ | ✅ | yes |
| 2 | `rico:DateRange` | 5,081,934 | — | — | yes |
| 3 | `rico:RecordSet` | 4,818,811 | — | — | yes |
| 4 | `schema:PropertyValue` | 2,412,516 | — | — | yes |
| 5 | `dwc:MaterialCitation` | 1,523,636 | ✅ | ✅ | yes |
| 6 | `schema:DefinedTerm` | 1,381,365 | ✅ | ✅ | yes |
| 7 | `dwcFP:TaxonConcept` | 993,243 | ✅ | ✅ | yes |
| 8 | `dwcFP:TaxonName` | 918,574 | ✅ | ✅ | yes |
| 9 | `treat:Treatment` | 871,356 | ✅ | ✅ | yes |
| 10 | `schema:PostalAddress` | 828,573 | — | — | yes |
| 11 | `schema:Organization` | 818,880 | ✅ | — | yes |
| 12 | `schch:ZefixOrganisation` | 792,332 | ✅ | ✅ | yes |
| 13 | `locn:Address` | 792,332 | — | — | yes |
| 14 | `https://lod.opentransportdata.swiss/vocab/Relation` | 577,343 | — | — | yes |
| 15 | `skos:Concept` | 458,236 | ✅ | ✅ | yes |
| 16 | `fabio:Figure` | 423,081 | ✅ | ✅ | yes |
| 17 | `https://www.w3.org/ns/oa#Annotation` | 397,886 | — | — | yes |
| 18 | `vl:Version` | 366,253 | — | — | yes |
| 19 | `rico:Record` | 263,123 | — | — | yes |
| 20 | `vl:Identity` | 158,435 | — | — | yes |
| 21 | `https://lod.opentransportdata.swiss/vocab/TransportEdge` | 156,985 | — | — | yes |
| 22 | `fabio:JournalArticle` | 84,934 | ✅ | ✅ | yes |
| 23 | `schch:Term` | 77,865 | ✅ | ✅ | yes |
| 24 | `schch:Synonym` | 63,139 | — | — | yes |
| 25 | `geo:Geometry` | 54,341 | — | — | yes |
| 26 | `schema:CivicStructure` | 54,115 | — | — | yes |
| 27 | `gtfs:Station` | 54,115 | — | — | yes |
| 28 | `rico:Instantiation` | 41,976 | — | — | yes |
| 29 | `schema:Offer` | 40,291 | — | — | yes |
| 30 | `vl:Deprecated` | 37,502 | — | — | yes |
| 31 | `schch:ValidatedEntry` | 35,249 | — | — | yes |
| 32 | `https://lod.opentransportdata.swiss/vocab/ZoningPriceCharacteristic` | 23,859 | — | — | yes |
| 33 | `schema:QuantitativeValue` | 21,933 | — | — | yes |
| 34 | `foaf:Agent` | 20,996 | — | — | yes |
| 35 | `foaf:Organization` | 20,427 | — | — | yes |
| 36 | `rico:Activity` | 17,657 | — | — | yes |
| 37 | `schch:Name` | 16,420 | — | — | yes |
| 38 | `schema:GovernmentOrganization` | 12,292 | — | — | yes |
| 39 | `schema:Person` | 12,291 | ✅ | — | yes |
| 40 | `http://www.w3.org/2006/time#Instant` | 11,211 | — | — | yes |
| 41 | `schch:Abbreviation` | 8,918 | — | — | yes |
| 42 | `schema:Role` | 8,627 | — | — | yes |
| 43 | `rico:RecordResourceToRecordResourceRelation` | 8,598 | — | — | yes |
| 44 | `rdf:List` | 8,130 | — | — | yes |
| 45 | `https://agriculture.ld.admin.ch/plant-protection/Indication` | 7,878 | — | — | yes |
| 46 | `cube:KeyDimension` | 6,745 | — | — | yes |
| 47 | `http://www.w3.org/2006/time#Interval` | 6,313 | — | — | yes |
| 48 | `https://lod.opentransportdata.swiss/vocab/Tarifwertpreisauspraegung` | 6,131 | — | — | yes |
| 49 | `vl:ChangeEvent` | 6,099 | — | — | yes |
| 50 | `https://ld.admin.ch/ech/71/MunicipalityVersion` | 5,868 | — | — | yes |
| 51 | `schch:MunicipalityChangeEvent` | 5,845 | — | — | yes |
| 52 | `http://www.w3.org/2006/time#ProperInterval` | 5,765 | — | — | yes |
| 53 | `qudt:FactorUnit` | 4,871 | — | — | yes |
| 54 | `https://lod.opentransportdata.swiss/vocab/PayLevel` | 4,735 | — | — | yes |
| 55 | `schch:Phraseology` | 4,394 | — | — | yes |
| 56 | `schema:CreativeWork` | 4,230 | — | — | yes |
| 57 | `schch:InProgressEntry` | 4,195 | — | — | yes |
| 58 | `https://agriculture.ld.admin.ch/plant-protection/Ingredient` | 4,078 | — | — | yes |
| 59 | `cube:MeasureDimension` | 4,057 | — | — | yes |
| 60 | `https://agriculture.ld.admin.ch/inspection/InspectionPoint` | 3,952 | — | — | yes |
| 61 | `schema:OrganizationRole` | 3,569 | — | — | yes |
| 62 | `schch:Municipality` | 3,454 | ✅ | ✅ | yes |
| 63 | `https://ld.admin.ch/ech/71/Municipality` | 3,454 | — | — | yes |
| 64 | `https://lod.opentransportdata.swiss/vocab/Relationsgebiet` | 3,454 | — | — | yes |
| 65 | `vl:InitialRecording` | 3,340 | — | — | yes |
| 66 | `schch:PoliticalMunicipality` | 3,250 | — | — | yes |
| 67 | `http://purl.org/linked-data/cube#Observation` | 3,229 | — | — | yes |
| 68 | `https://lod.opentransportdata.swiss/vocab/Zone` | 3,188 | — | — | yes |
| 69 | `qudt:Unit` | 2,992 | — | — | yes |
| 70 | `https://ld.admin.ch/ech/71/MunicipalityChangeEvent` | 2,703 | — | — | yes |
| 71 | `https://agriculture.ld.admin.ch/plant-protection/Product` | 2,384 | — | — | yes |
| 72 | `http://www.w3.org/ns/prov#Association` | 2,373 | — | — | yes |
| 73 | `dcat:Dataset` | 2,126 | — | — | yes |
| 74 | `schch:Isil` | 2,118 | — | — | yes |
| 75 | `sh:NodeShape` | 2,083 | — | — | yes |
| 76 | `rdf:Property` | 2,076 | ✅ | — | yes |
| 77 | `vcard:Organization` | 2,029 | — | — | yes |
| 78 | `cube:Cube` | 2,028 | ✅ | ✅ | yes |
| 79 | `cube:Constraint` | 2,024 | ✅ | ✅ | yes |
| 80 | `cube:ObservationSet` | 1,994 | ✅ | ✅ | yes |
| 81 | `schema:ContactPoint` | 1,992 | — | — | yes |
| 82 | `meta:Hierarchy` | 1,991 | ✅ | ✅ | yes |
| 83 | `void:Dataset` | 1,983 | — | — | yes |
| 84 | `schema:LobbyOrganization` | 1,982 | — | — | yes |
| 85 | `schema:Dataset` | 1,943 | — | — | yes |
| 86 | `dct:Standard` | 1,901 | — | — | yes |
| 87 | `dct:MediaType` | 1,901 | — | — | yes |
| 88 | `http://rdf-vocabulary.ddialliance.org/xkos#ClassificationLevel` | 1,646 | — | — | yes |
| 89 | `owl:NamedIndividual` | 1,638 | — | — | yes |
| 90 | `https://ch.paf.link/ProceduralRequestInformationActivity` | 1,604 | — | — | yes |
| 91 | `http://www.w3.org/2006/time#GeneralDateTimeDescription` | 1,515 | — | — | yes |
| 92 | `vl:ChangeInHierarchy` | 1,498 | — | — | yes |
| 93 | `https://agriculture.ld.admin.ch/foag/Product` | 1,485 | — | — | yes |
| 94 | `https://agriculture.ld.admin.ch/plant-protection/Obligation` | 1,476 | — | — | yes |
| 95 | `owl:Class` | 1,415 | ✅ | — | yes |
| 96 | `rdfs:Class` | 1,334 | ✅ | ✅ | yes |
| 97 | `https://ch.paf.link/ParliamentaryAffairIdentifierEntity` | 1,234 | — | — | yes |
| 98 | `https://ch.paf.link/ProceduralRequestEntity` | 1,234 | — | — | yes |
| 99 | `qudt:QuantityKind` | 1,223 | — | — | yes |
| 100 | `fabio:BookSection` | 1,180 | ✅ | ✅ | yes |
| 101 | `https://ch.paf.link/ProceduralRequestInformationEntity` | 1,174 | — | — | yes |
| 102 | `https://agriculture.ld.admin.ch/plant-protection/RegularProduct` | 1,120 | — | — | yes |
| 103 | `relation:StandardError` | 1,026 | — | — | yes |
| 104 | `schema:DefinedTermSet` | 966 | ✅ | ✅ | yes |
| 105 | `https://agriculture.ld.admin.ch/plant-protection/ApplicationComment` | 943 | — | — | yes |
| 106 | `dct:PeriodOfTime` | 887 | — | — | yes |
| 107 | `schema:GeoShape` | 842 | — | — | yes |
| 108 | `https://agriculture.ld.admin.ch/plant-protection/Herbicide` | 840 | — | — | yes |
| 109 | `https://lod.opentransportdata.swiss/vocab/Tarif` | 837 | — | — | yes |
| 110 | `https://ch.paf.link/ProceduralRequestProposalActivity` | 769 | — | — | yes |
| 111 | `https://agriculture.ld.admin.ch/eCH-0265/2/CultivationType` | 755 | — | — | yes |
| 112 | `skos:ConceptScheme` | 720 | ✅ | ✅ | yes |
| 113 | `dct:Collection` | 716 | — | — | yes |
| 114 | `https://agriculture.ld.admin.ch/plant-protection/Fungicide` | 688 | — | — | yes |
| 115 | `owl:ObjectProperty` | 671 | — | — | yes |
| 116 | `https://agriculture.ld.admin.ch/plant-protection/ParallelImport` | 643 | — | — | yes |
| 117 | `https://ch.paf.link/ProceduralRequestProposalEntity` | 635 | — | — | yes |
| 118 | `https://agriculture.ld.admin.ch/plant-protection/SalePermission` | 621 | — | — | yes |
| 119 | `owl:Restriction` | 603 | — | — | yes |
| 120 | `https://ch.paf.link/ProceduralRequestConnex` | 564 | — | — | yes |
| 121 | `https://agriculture.ld.admin.ch/plant-protection/Pest` | 527 | — | — | yes |
| 122 | `http://www.w3.org/ns/hydra/core#Resource` | 513 | — | — | yes |
| 123 | `shdim:SharedDimensionTerm` | 464 | ✅ | ✅ | yes |
| 124 | `dcat:Relationship` | 455 | — | — | yes |
| 125 | `https://agriculture.ld.admin.ch/plant-protection/Substance` | 453 | — | — | yes |
| 126 | `qudt:DerivedUnit` | 393 | — | — | yes |
| 127 | `https://agriculture.ld.admin.ch/plant-protection/Insecticide` | 369 | — | — | yes |
| 128 | `qudt:CurrencyUnit` | 360 | — | — | yes |
| 129 | `euvoc:Country` | 345 | — | — | yes |
| 130 | `https://agriculture.ld.admin.ch/plant-protection/ActiveSubstance` | 339 | — | — | yes |
| 131 | `https://lod.opentransportdata.swiss/vocab/Preistabelle` | 334 | — | — | yes |
| 132 | `qudt:PhysicalConstant` | 331 | — | — | yes |
| 133 | `qudt:ConstantValue` | 331 | — | — | yes |
| 134 | `schema:AdministrativeArea` | 327 | — | — | yes |
| 135 | `schema:Event` | 326 | — | — | yes |
| 136 | `https://agriculture.ld.admin.ch/eCH-0265/2/PlantProtectionCrop` | 326 | — | — | yes |
| 137 | `https://agriculture.ld.admin.ch/plant-protection/Crop` | 326 | — | — | yes |
| 138 | `https://agriculture.ld.admin.ch/eCH-0265/2/NutrientBalanceCrop` | 314 | — | — | yes |
| 139 | `https://agriculture.ld.admin.ch/plant-protection/Acaricide` | 312 | — | — | yes |
| 140 | `https://ld.admin.ch/ech/71/DistrictVersion` | 309 | — | — | yes |
| 141 | `schema:Country` | 284 | ✅ | ✅ | yes |
| 142 | `schch:District` | 265 | ✅ | ✅ | yes |
| 143 | `https://ld.admin.ch/ech/71/District` | 265 | — | — | yes |
| 144 | `schch:DistrictChangeEvent` | 254 | — | — | yes |
| 145 | `qudt:QuantityKindDimensionVector` | 247 | — | — | yes |
| 146 | `https://agriculture.ld.admin.ch/foag/ProductSubgroup` | 246 | — | — | yes |
| 147 | `http://example.com/HydroMeasuringStation` | 233 | — | — | yes |
| 148 | `euvoc:FileType` | 228 | — | — | yes |
| 149 | `schch:TermSubDomain` | 226 | — | — | yes |
| 150 | `qudt:QuantityKindDimensionVector_SI` | 225 | — | — | yes |
| 151 | `qudt:QuantityKindDimensionVector_ISO` | 221 | — | — | yes |
| 152 | `qudt:QuantityKindDimensionVector_Imperial` | 221 | — | — | yes |
| 153 | `owl:DatatypeProperty` | 214 | — | — | yes |
| 154 | `dcat:Distribution` | 197 | ✅ | ✅ | yes |
| 155 | `schema:ParliamentaryCommittee` | 186 | — | — | yes |
| 156 | `https://agriculture.ld.admin.ch/eCH-0265/2/DirectPaymentCrop` | 182 | — | — | yes |
| 157 | `schch:Session` | 181 | — | — | yes |
| 158 | `https://lod.opentransportdata.swiss/vocab/Anwendungsbereich` | 165 | — | — | yes |
| 159 | `schema:BodyOfWater` | 162 | — | — | yes |
| 160 | `fabio:Book` | 160 | ✅ | ✅ | yes |
| 161 | `sh:PropertyShape` | 160 | — | — | yes |
| 162 | `owl:AnnotationProperty` | 154 | — | — | yes |
| 163 | `https://lod.opentransportdata.swiss/vocab/LocalNetwork` | 150 | — | — | yes |
| 164 | `schema:SoftwareApplication` | 150 | — | — | yes |
| 165 | `genid-b6e190ffca0942d28032a27ec4f915391681662-B1CA8D76547D0FCC5B64F8509C69302C` | 150 | — | — | yes |
| 166 | `genid-b6e190ffca0942d28032a27ec4f915391681662-7DB9868683ED71F8CB39ADD5913D6BAD` | 150 | — | — | yes |
| 167 | `https://agriculture.ld.admin.ch/plant-protection/PlantGrowthRegulator` | 145 | — | — | yes |
| 168 | `https://environment.ld.admin.ch/foen/gefahren-waldbrand/Region` | 143 | — | — | yes |
| 169 | `http://www.w3.org/2011/content#ContentAsText` | 142 | — | — | yes |
| 170 | `genid-b6e190ffca0942d28032a27ec4f915391681662-D0DEFE4EB3BFF0EB57AB219BF1C6058E` | 121 | — | — | yes |
| 171 | `genid-b6e190ffca0942d28032a27ec4f915391681662-A6C91639A6E3E4BC8E83684D78CF9710` | 121 | — | — | yes |
| 172 | `genid-b6e190ffca0942d28032a27ec4f915391681662-C6DCFE0BE13BDD1C4A7AA77B25F6E1AA` | 121 | — | — | yes |
| 173 | `https://agriculture.ld.admin.ch/plant-protection/BeneficialInsectAgent` | 121 | — | — | yes |
| 174 | `schch:TerminologyCollection` | 107 | — | — | yes |
| 175 | `owl:FunctionalProperty` | 104 | — | — | yes |
| 176 | `qudt:QuantityKindDimensionVector_CGS` | 101 | — | — | yes |
| 177 | `meta:SharedDimension` | 96 | ✅ | ✅ | yes |
| 178 | `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/2777` | 95 | — | — | yes |
| 179 | `schema:webpage` | 95 | — | — | yes |
| 180 | `schch:ChemicalElement` | 92 | — | — | yes |
| 181 | `schema:DigitalDocument` | 91 | — | — | yes |
| 182 | `schema:PoliticalParty` | 82 | — | — | yes |
| 183 | `https://lod.opentransportdata.swiss/vocab/ZoningPlan` | 82 | — | — | yes |
| 184 | `vl:ChangeOfName` | 80 | — | — | yes |
| 185 | `https://agriculture.ld.admin.ch/plant-protection/GHSLabelElement` | 74 | — | — | yes |
| 186 | `https://agriculture.ld.admin.ch/system-map/PrivateOrganization` | 72 | — | — | yes |
| 187 | `qudt:ContextualUnit` | 71 | — | — | yes |
| 188 | `https://agriculture.ld.admin.ch/plant-protection/Molluscicide` | 68 | — | — | yes |
| 189 | `https://agriculture.ld.admin.ch/foag/DataSource` | 66 | — | — | yes |
| 190 | `https://agriculture.ld.admin.ch/plant-protection/HazardStatement` | 64 | — | — | yes |
| 191 | `cube:AttributeDimension` | 64 | — | — | yes |
| 192 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2207` | 58 | — | — | yes |
| 193 | `https://ld.admin.ch/ech/71/DistrictChangeEvent` | 58 | — | — | yes |
| 194 | `https://agriculture.ld.admin.ch/system-map/CantonalOrganization` | 51 | — | — | yes |
| 195 | `https://environment.ld.admin.ch/foen/nfi/Unit` | 50 | — | — | yes |
| 196 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1030` | 50 | — | — | yes |
| 197 | `qudt:CountingUnit` | 50 | — | — | yes |
| 198 | `https://agriculture.ld.admin.ch/foag/ProductGroup` | 49 | — | — | no |
| 199 | `sh:SPARQLTarget` | 49 | — | — | no |
| 200 | `rdfs:Datatype` | 46 | — | — | no |
| 201 | `https://agriculture.ld.admin.ch/plant-protection/WettingAndAdhesionAgent` | 44 | — | — | no |
| 202 | `schema:MedicalSpecialty` | 42 | — | — | no |
| 203 | `https://agriculture.ld.admin.ch/system-map/FederalOrganization` | 41 | — | — | no |
| 204 | `schch:TermDomain` | 41 | — | — | no |
| 205 | `euvoc:Frequency` | 41 | — | — | no |
| 206 | `schch:CantonalLakePortion` | 41 | — | — | no |
| 207 | `https://agriculture.ld.admin.ch/plant-protection/BeneficialNematodeAgent` | 37 | — | — | no |
| 208 | `schema:USNonprofitType` | 36 | — | — | no |
| 209 | `https://agriculture.ld.admin.ch/plant-protection/Pheromone` | 36 | — | — | no |
| 210 | `https://lod.opentransportdata.swiss/vocab/Alliance` | 35 | — | — | no |
| 211 | `https://agriculture.ld.admin.ch/plant-protection/SeedTreatmentProduct` | 34 | — | — | no |
| 212 | `qudt:Prefix` | 33 | — | — | no |
| 213 | `owl:Ontology` | 31 | — | — | no |
| 214 | `schema:ParliamentaryGroup` | 31 | — | — | no |
| 215 | `https://agriculture.ld.admin.ch/foag/DataMethod` | 31 | — | — | no |
| 216 | `https://agriculture.ld.admin.ch/plant-protection/BeneficialMiteAgent` | 31 | — | — | no |
| 217 | `https://agriculture.ld.admin.ch/foag/Unit` | 30 | — | — | no |
| 218 | `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/366` | 30 | — | — | no |
| 219 | `schema:HealthAspectEnumeration` | 29 | — | — | no |
| 220 | `http://www.w3.org/shacl#NodeShape` | 29 | — | — | no |
| 221 | `schema:GeoCoordinates` | 29 | — | — | no |
| 222 | `https://agriculture.ld.admin.ch/foag/ProductOrigin` | 28 | — | — | no |
| 223 | `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/827` | 28 | — | — | no |
| 224 | `https://agriculture.ld.admin.ch/plant-protection/PlantProtectionStatement` | 27 | — | — | no |
| 225 | `schch:Canton` | 26 | ✅ | ✅ | no |
| 226 | `schema:State` | 26 | — | — | no |
| 227 | `https://ld.admin.ch/ech/71/Canton` | 26 | — | — | no |
| 228 | `https://register.ld.admin.ch/termdat/52451` | 26 | — | — | no |
| 229 | `https://agriculture.ld.admin.ch/foag/SalesRegion` | 25 | — | — | no |
| 230 | `https://agriculture.ld.admin.ch/foag/ValueChain_Detail` | 25 | — | — | no |
| 231 | `qudt:DecimalPrefix` | 25 | — | — | no |
| 232 | `https://lod.opentransportdata.swiss/vocab/VorberechnetePreistabelle` | 25 | — | — | no |
| 233 | `https://agriculture.ld.admin.ch/foag/ProductionSystem` | 23 | — | — | no |
| 234 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2360` | 22 | — | — | no |
| 235 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/720` | 22 | — | — | no |
| 236 | `owl:AllDisjointClasses` | 22 | — | — | no |
| 237 | `shdim:SharedDimension` | 21 | — | — | no |
| 238 | `https://agriculture.ld.admin.ch/foag/Usage` | 21 | — | — | no |
| 239 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1002` | 21 | — | — | no |
| 240 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/597` | 20 | — | — | no |
| 241 | `https://agriculture.ld.admin.ch/plant-protection/StorageProtectionProduct` | 20 | — | — | no |
| 242 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/501` | 19 | — | — | no |
| 243 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/704` | 19 | — | — | no |
| 244 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/709` | 19 | — | — | no |
| 245 | `schema:Legislation` | 19 | — | — | no |
| 246 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2619` | 18 | — | — | no |
| 247 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2679` | 18 | — | — | no |
| 248 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/90` | 17 | — | — | no |
| 249 | `schema:WearableSizeGroupEnumeration` | 17 | — | — | no |
| 250 | `schema:IPTCDigitalSourceEnumeration` | 17 | — | — | no |
| 251 | `owl:SymmetricProperty` | 16 | — | — | no |
| 252 | `schch:LegislaturePeriod` | 16 | — | — | no |
| 253 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2004` | 16 | — | — | no |
| 254 | `shdim:Hierarchy` | 15 | — | — | no |
| 255 | `schema:UpdateAction` | 15 | — | — | no |
| 256 | `https://agriculture.ld.admin.ch/foag/ProductProperties` | 15 | — | — | no |
| 257 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2625` | 15 | — | — | no |
| 258 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2667` | 15 | — | — | no |
| 259 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/762` | 15 | — | — | no |
| 260 | `euvoc:DataTheme` | 14 | — | — | no |
| 261 | `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/829` | 14 | — | — | no |
| 262 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/898` | 14 | — | — | no |
| 263 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1047` | 14 | — | — | no |
| 264 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1263` | 14 | — | — | no |
| 265 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2392` | 14 | — | — | no |
| 266 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/385` | 14 | — | — | no |
| 267 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/564` | 14 | — | — | no |
| 268 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2695` | 14 | — | — | no |
| 269 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2755` | 14 | — | — | no |
| 270 | `schema:PhysicalExam` | 14 | — | — | no |
| 271 | `schema:WearableSizeSystemEnumeration` | 14 | — | — | no |
| 272 | `https://agriculture.ld.admin.ch/plant-protection/FungalBiologicalAgent` | 14 | — | — | no |
| 273 | `https://agriculture.ld.admin.ch/plant-protection/InsectVirusAgent` | 14 | — | — | no |
| 274 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/582` | 13 | — | — | no |
| 275 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/601` | 13 | — | — | no |
| 276 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2756` | 13 | — | — | no |
| 277 | `schema:BodyMeasurementTypeEnumeration` | 13 | — | — | no |
| 278 | `http://purl.org/linked-data/cube#DataSet` | 13 | — | — | no |
| 279 | `http://purl.org/linked-data/cube#DataStructureDefinition` | 13 | — | — | no |
| 280 | `rico:RecordSetType` | 12 | — | — | no |
| 281 | `void:DatasetDescription` | 12 | — | — | no |
| 282 | `schema:DataCatalog` | 12 | — | — | no |
| 283 | `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/49` | 12 | — | — | no |
| 284 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2632` | 12 | — | — | no |
| 285 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1310` | 12 | — | — | no |
| 286 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1317` | 12 | — | — | no |
| 287 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2637` | 12 | — | — | no |
| 288 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2666` | 12 | — | — | no |
| 289 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/585` | 12 | — | — | no |
| 290 | `schema:ItemAvailability` | 12 | — | — | no |
| 291 | `schema:WearableMeasurementTypeEnumeration` | 12 | — | — | no |
| 292 | `schch:Glossary` | 11 | — | — | no |
| 293 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1034` | 11 | — | — | no |
| 294 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1302` | 11 | — | — | no |
| 295 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/599` | 11 | — | — | no |
| 296 | `schema:City` | 11 | — | — | no |
| 297 | `schch:ForeignMunicipality` | 11 | — | — | no |
| 298 | `schema:RestrictedDiet` | 11 | — | — | no |
| 299 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1316` | 10 | — | — | no |
| 300 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2583` | 10 | — | — | no |
| 301 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2630` | 10 | — | — | no |
| 302 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2673` | 10 | — | — | no |
| 303 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/460` | 10 | — | — | no |
| 304 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/563` | 10 | — | — | no |
| 305 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/581` | 10 | — | — | no |
| 306 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/600` | 10 | — | — | no |
| 307 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/84` | 10 | — | — | no |
| 308 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/964` | 10 | — | — | no |
| 309 | `qudt:SystemOfUnits` | 10 | — | — | no |
| 310 | `schema:MedicalStudyStatus` | 10 | — | — | no |
| 311 | `schema:EUEnergyEfficiencyEnumeration` | 10 | — | — | no |
| 312 | `schema:AdultOrientedEnumeration` | 10 | — | — | no |
| 313 | `https://lod.opentransportdata.swiss/vocab/CustomerSegment` | 10 | — | — | no |
| 314 | `https://agriculture.ld.admin.ch/system-map/FMIS` | 10 | — | — | no |
| 315 | `http://purl.org/ontology/service#Service` | 10 | — | — | no |
| 316 | `genid-b6e190ffca0942d28032a27ec4f915391681662-86117D87934284C52A9662AF3B43776D` | 10 | — | — | no |
| 317 | `genid-b6e190ffca0942d28032a27ec4f915391681662-C46B6F9503515DF48EE1F8C03F29C997` | 10 | — | — | no |
| 318 | `https://agriculture.ld.admin.ch/plant-protection/ApplicationArea` | 10 | — | — | no |
| 319 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/96` | 9 | — | — | no |
| 320 | `https://agriculture.ld.admin.ch/foag/CostComponent` | 9 | — | — | no |
| 321 | `https://agriculture.ld.admin.ch/foag/KeyIndicatorType` | 9 | — | — | no |
| 322 | `https://agriculture.ld.admin.ch/foag/Market` | 9 | — | — | no |
| 323 | `https://agriculture.ld.admin.ch/foag/ValueChain` | 9 | — | — | no |
| 324 | `https://environment.ld.admin.ch/foen/nfi/Inventory` | 9 | — | — | no |
| 325 | `http://www.w3.org/2006/time#TemporalEntity` | 9 | — | — | no |
| 326 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1256` | 9 | — | — | no |
| 327 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1264` | 9 | — | — | no |
| 328 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1925` | 9 | — | — | no |
| 329 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1938` | 9 | — | — | no |
| 330 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1939` | 9 | — | — | no |
| 331 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/23` | 9 | — | — | no |
| 332 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/28` | 9 | — | — | no |
| 333 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/449` | 9 | — | — | no |
| 334 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/456` | 9 | — | — | no |
| 335 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/579` | 9 | — | — | no |
| 336 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/636` | 9 | — | — | no |
| 337 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/689` | 9 | — | — | no |
| 338 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/711` | 9 | — | — | no |
| 339 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/763` | 9 | — | — | no |
| 340 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/930` | 9 | — | — | no |
| 341 | `qudt:DimensionlessUnit` | 9 | — | — | no |
| 342 | `http://www.linkedmodel.org/schema/vaem#GraphMetaData` | 9 | — | — | no |
| 343 | `schema:MusicAlbumProductionType` | 9 | — | — | no |
| 344 | `schema:MedicalTrialDesign` | 9 | — | — | no |
| 345 | `https://agriculture.ld.admin.ch/plant-protection/Rodenticide` | 9 | — | — | no |
| 346 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1157` | 8 | — | — | no |
| 347 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/20` | 8 | — | — | no |
| 348 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2633` | 8 | — | — | no |
| 349 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1027` | 8 | — | — | no |
| 350 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1083` | 8 | — | — | no |
| 351 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1308` | 8 | — | — | no |
| 352 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1335` | 8 | — | — | no |
| 353 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1805` | 8 | — | — | no |
| 354 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1831` | 8 | — | — | no |
| 355 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1986` | 8 | — | — | no |
| 356 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/454` | 8 | — | — | no |
| 357 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/508` | 8 | — | — | no |
| 358 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/593` | 8 | — | — | no |
| 359 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/594` | 8 | — | — | no |
| 360 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/602` | 8 | — | — | no |
| 361 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/690` | 8 | — | — | no |
| 362 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/868` | 8 | — | — | no |
| 363 | `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/2586` | 8 | — | — | no |
| 364 | `meta:DimensionRelation` | 8 | — | — | no |
| 365 | `schema:GovernmentBenefitsType` | 8 | — | — | no |
| 366 | `schema:DayOfWeek` | 8 | — | — | no |
| 367 | `schema:PriceTypeEnumeration` | 8 | — | — | no |
| 368 | `schema:OrderStatus` | 8 | — | — | no |
| 369 | `qudt:BinaryPrefix` | 8 | — | — | no |
| 370 | `schema:PaymentMethodType` | 8 | — | — | no |
| 371 | `https://agriculture.ld.admin.ch/plant-protection/AntifungalBiologicalAgent` | 8 | — | — | no |
| 372 | `https://agriculture.ld.admin.ch/plant-protection/HazardPictogram` | 8 | — | — | no |
| 373 | `https://agriculture.ld.admin.ch/eCH-0265/2/NutrientBalanceCropSubCategory` | 8 | — | — | no |
| 374 | `owl:Axiom` | 8 | — | — | no |
| 375 | `http://www.w3.org/2006/time#TemporalUnit` | 7 | — | — | no |
| 376 | `https://agriculture.ld.admin.ch/foag/Currency` | 7 | — | — | no |
| 377 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/963` | 7 | — | — | no |
| 378 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1029` | 7 | — | — | no |
| 379 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1033` | 7 | — | — | no |
| 380 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1262` | 7 | — | — | no |
| 381 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1287` | 7 | — | — | no |
| 382 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1294` | 7 | — | — | no |
| 383 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1299` | 7 | — | — | no |
| 384 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1336` | 7 | — | — | no |
| 385 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1338` | 7 | — | — | no |
| 386 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2011` | 7 | — | — | no |
| 387 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2171` | 7 | — | — | no |
| 388 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2221` | 7 | — | — | no |
| 389 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2325` | 7 | — | — | no |
| 390 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2369` | 7 | — | — | no |
| 391 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2398` | 7 | — | — | no |
| 392 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2540` | 7 | — | — | no |
| 393 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2623` | 7 | — | — | no |
| 394 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2647` | 7 | — | — | no |
| 395 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/33` | 7 | — | — | no |
| 396 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/450` | 7 | — | — | no |
| 397 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/453` | 7 | — | — | no |
| 398 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/583` | 7 | — | — | no |
| 399 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/688` | 7 | — | — | no |
| 400 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/706` | 7 | — | — | no |
| 401 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/917` | 7 | — | — | no |
| 402 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/98` | 7 | — | — | no |
| 403 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2691` | 7 | — | — | no |
| 404 | `schema:TherapeuticProcedure` | 7 | — | — | no |
| 405 | `schema:DataType` | 7 | — | — | no |
| 406 | `schema:PhysicalActivityCategory` | 7 | — | — | no |
| 407 | `schema:MusicReleaseFormatType` | 7 | — | — | no |
| 408 | `http://www.w3.org/2006/time#DayOfWeek` | 7 | — | — | no |
| 409 | `euvoc:Continent` | 7 | — | — | no |
| 410 | `schema:DENonprofitType` | 7 | — | — | no |
| 411 | `https://agriculture.ld.admin.ch/plant-protection/CoFormulant` | 7 | — | — | no |
| 412 | `https://agriculture.ld.admin.ch/eCH-0265/2/DirectPaymentAreaCategory` | 7 | — | — | no |
| 413 | `dcat:PeriodOfTime` | 7 | — | — | no |
| 414 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/94` | 6 | — | — | no |
| 415 | `schch:TerminologyOffice` | 6 | — | — | no |
| 416 | `schch:MunicipalityFreeArea` | 6 | — | — | no |
| 417 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/22` | 6 | — | — | no |
| 418 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/894` | 6 | — | — | no |
| 419 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1003` | 6 | — | — | no |
| 420 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1088` | 6 | — | — | no |
| 421 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1257` | 6 | — | — | no |
| 422 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1290` | 6 | — | — | no |
| 423 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1328` | 6 | — | — | no |
| 424 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1788` | 6 | — | — | no |
| 425 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/19` | 6 | — | — | no |
| 426 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1923` | 6 | — | — | no |
| 427 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1924` | 6 | — | — | no |
| 428 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1928` | 6 | — | — | no |
| 429 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2345` | 6 | — | — | no |
| 430 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2405` | 6 | — | — | no |
| 431 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2678` | 6 | — | — | no |
| 432 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/329` | 6 | — | — | no |
| 433 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/505` | 6 | — | — | no |
| 434 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/584` | 6 | — | — | no |
| 435 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/598` | 6 | — | — | no |
| 436 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/606` | 6 | — | — | no |
| 437 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/653` | 6 | — | — | no |
| 438 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/691` | 6 | — | — | no |
| 439 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/729` | 6 | — | — | no |
| 440 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/732` | 6 | — | — | no |
| 441 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/757` | 6 | — | — | no |
| 442 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2239` | 6 | — | — | no |
| 443 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2757` | 6 | — | — | no |
| 444 | `schema:PriceComponentTypeEnumeration` | 6 | — | — | no |
| 445 | `schema:BookFormatType` | 6 | — | — | no |
| 446 | `schema:MedicineSystem` | 6 | — | — | no |
| 447 | `schema:InfectiousAgentClass` | 6 | — | — | no |
| 448 | `schema:MedicalImagingTechnique` | 6 | — | — | no |
| 449 | `schema:MedicalObservationalStudyDesign` | 6 | — | — | no |
| 450 | `schema:DrugPregnancyCategory` | 6 | — | — | no |
| 451 | `schema:MediaManipulationRatingEnumeration` | 6 | — | — | no |
| 452 | `schema:Continent` | 6 | — | — | no |
| 453 | `https://agriculture.ld.admin.ch/system-map/MasterData` | 6 | — | — | no |
| 454 | `schema:ITNonprofitType` | 6 | — | — | no |
| 455 | `https://agriculture.ld.admin.ch/plant-protection/ChemicalPlantProtectionProduct` | 6 | — | — | no |
| 456 | `https://agriculture.ld.admin.ch/plant-protection/Disinfectant` | 6 | — | — | no |
| 457 | `https://agriculture.ld.admin.ch/plant-protection/PlantDefenseInducer` | 6 | — | — | no |
| 458 | `schch:LindasDataset` | 5 | — | — | no |
| 459 | `owl:TransitiveProperty` | 5 | — | — | no |
| 460 | `https://ch.paf.link/ProceduralRequestReportActivity` | 5 | — | — | no |
| 461 | `https://ch.paf.link/ProceduralRequestReportEntity` | 5 | — | — | no |
| 462 | `https://agriculture.ld.admin.ch/foag/ForeignTrade` | 5 | — | — | no |
| 463 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1084` | 5 | — | — | no |
| 464 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1280` | 5 | — | — | no |
| 465 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1284` | 5 | — | — | no |
| 466 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1291` | 5 | — | — | no |
| 467 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1295` | 5 | — | — | no |
| 468 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1301` | 5 | — | — | no |
| 469 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1304` | 5 | — | — | no |
| 470 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1307` | 5 | — | — | no |
| 471 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1311` | 5 | — | — | no |
| 472 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1323` | 5 | — | — | no |
| 473 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1326` | 5 | — | — | no |
| 474 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2346` | 5 | — | — | no |
| 475 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2368` | 5 | — | — | no |
| 476 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2404` | 5 | — | — | no |
| 477 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2599` | 5 | — | — | no |
| 478 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2631` | 5 | — | — | no |
| 479 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2668` | 5 | — | — | no |
| 480 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/320` | 5 | — | — | no |
| 481 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/511` | 5 | — | — | no |
| 482 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/692` | 5 | — | — | no |
| 483 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/723` | 5 | — | — | no |
| 484 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/758` | 5 | — | — | no |
| 485 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/852` | 5 | — | — | no |
| 486 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/887` | 5 | — | — | no |
| 487 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/891` | 5 | — | — | no |
| 488 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1870` | 5 | — | — | no |
| 489 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2694` | 5 | — | — | no |
| 490 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2707` | 5 | — | — | no |
| 491 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2238` | 5 | — | — | no |
| 492 | `http://www.w3.org/2006/time#DurationDescription` | 5 | — | — | no |
| 493 | `schema:EventStatusType` | 5 | — | — | no |
| 494 | `schema:ReturnFeesEnumeration` | 5 | — | — | no |
| 495 | `schema:PaymentStatusType` | 5 | — | — | no |
| 496 | `http://www.linkedmodel.org/schema/vaem#CatalogEntry` | 5 | — | — | no |
| 497 | `qudt:LogarithmicUnit` | 5 | — | — | no |
| 498 | `qudt:List` | 5 | — | — | no |
| 499 | `euvoc:PlannedAvailability` | 5 | — | — | no |
| 500 | `schema:DigitalPlatformEnumeration` | 5 | — | — | no |
| 501 | `schema:FulfillmentTypeEnumeration` | 5 | — | — | no |
| 502 | `schema:IncentiveType` | 5 | — | — | no |
| 503 | `http://purl.org/dc/dcmitype/Collection` | 5 | — | — | no |
| 504 | `dct:LicenseDocument` | 4 | — | — | no |
| 505 | `schema:Corporation` | 4 | — | — | no |
| 506 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2635` | 4 | — | — | no |
| 507 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1044` | 4 | — | — | no |
| 508 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1095` | 4 | — | — | no |
| 509 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1265` | 4 | — | — | no |
| 510 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1266` | 4 | — | — | no |
| 511 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1281` | 4 | — | — | no |
| 512 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1282` | 4 | — | — | no |
| 513 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1283` | 4 | — | — | no |
| 514 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1327` | 4 | — | — | no |
| 515 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1636` | 4 | — | — | no |
| 516 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1662` | 4 | — | — | no |
| 517 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1697` | 4 | — | — | no |
| 518 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1754` | 4 | — | — | no |
| 519 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1990` | 4 | — | — | no |
| 520 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2576` | 4 | — | — | no |
| 521 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/26` | 4 | — | — | no |
| 522 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2638` | 4 | — | — | no |
| 523 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2643` | 4 | — | — | no |
| 524 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2644` | 4 | — | — | no |
| 525 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2645` | 4 | — | — | no |
| 526 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2646` | 4 | — | — | no |
| 527 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2686` | 4 | — | — | no |
| 528 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/30` | 4 | — | — | no |
| 529 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/457` | 4 | — | — | no |
| 530 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/459` | 4 | — | — | no |
| 531 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/462` | 4 | — | — | no |
| 532 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/468` | 4 | — | — | no |
| 533 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/471` | 4 | — | — | no |
| 534 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/472` | 4 | — | — | no |
| 535 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/473` | 4 | — | — | no |
| 536 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/474` | 4 | — | — | no |
| 537 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/477` | 4 | — | — | no |
| 538 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/478` | 4 | — | — | no |
| 539 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/479` | 4 | — | — | no |
| 540 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/480` | 4 | — | — | no |
| 541 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/507` | 4 | — | — | no |
| 542 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/512` | 4 | — | — | no |
| 543 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/513` | 4 | — | — | no |
| 544 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/514` | 4 | — | — | no |
| 545 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/515` | 4 | — | — | no |
| 546 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/516` | 4 | — | — | no |
| 547 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/517` | 4 | — | — | no |
| 548 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/518` | 4 | — | — | no |
| 549 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/519` | 4 | — | — | no |
| 550 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/520` | 4 | — | — | no |
| 551 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/521` | 4 | — | — | no |
| 552 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/522` | 4 | — | — | no |
| 553 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/566` | 4 | — | — | no |
| 554 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/857` | 4 | — | — | no |
| 555 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/861` | 4 | — | — | no |
| 556 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/876` | 4 | — | — | no |
| 557 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2739` | 4 | — | — | no |
| 558 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2761` | 4 | — | — | no |
| 559 | `https://environment.ld.admin.ch/foen/nfi/Grid/1746` | 4 | — | — | no |
| 560 | `https://environment.ld.admin.ch/foen/nfi/Grid/410` | 4 | — | — | no |
| 561 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/1348` | 4 | — | — | no |
| 562 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/1429` | 4 | — | — | no |
| 563 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/1876` | 4 | — | — | no |
| 564 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2282` | 4 | — | — | no |
| 565 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2382` | 4 | — | — | no |
| 566 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2604` | 4 | — | — | no |
| 567 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2608` | 4 | — | — | no |
| 568 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2615` | 4 | — | — | no |
| 569 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2652` | 4 | — | — | no |
| 570 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2653` | 4 | — | — | no |
| 571 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2675` | 4 | — | — | no |
| 572 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2676` | 4 | — | — | no |
| 573 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2677` | 4 | — | — | no |
| 574 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2681` | 4 | — | — | no |
| 575 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2682` | 4 | — | — | no |
| 576 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2683` | 4 | — | — | no |
| 577 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2684` | 4 | — | — | no |
| 578 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/2685` | 4 | — | — | no |
| 579 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/434` | 4 | — | — | no |
| 580 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/435` | 4 | — | — | no |
| 581 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/484` | 4 | — | — | no |
| 582 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/531` | 4 | — | — | no |
| 583 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/533` | 4 | — | — | no |
| 584 | `https://environment.ld.admin.ch/foen/nfi/UnitOfEvaluation/828` | 4 | — | — | no |
| 585 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2629` | 4 | — | — | no |
| 586 | `https://environment.ld.admin.ch/foen/nfi/RegionType` | 4 | — | — | no |
| 587 | `schema:Consortium` | 4 | — | — | no |
| 588 | `schema:ActionStatusType` | 4 | — | — | no |
| 589 | `schema:MusicAlbumReleaseType` | 4 | — | — | no |
| 590 | `schema:DriveWheelConfigurationValue` | 4 | — | — | no |
| 591 | `schema:LegalValueLevel` | 4 | — | — | no |
| 592 | `schema:UKNonprofitType` | 4 | — | — | no |
| 593 | `schema:OfferItemCondition` | 4 | — | — | no |
| 594 | `schema:MerchantReturnEnumeration` | 4 | — | — | no |
| 595 | `schema:GameServerStatus` | 4 | — | — | no |
| 596 | `schema:MapCategoryType` | 4 | — | — | no |
| 597 | `schema:ReservationStatusType` | 4 | — | — | no |
| 598 | `schema:ReturnMethodEnumeration` | 4 | — | — | no |
| 599 | `schema:TierBenefitEnumeration` | 4 | — | — | no |
| 600 | `schema:IncentiveQualifiedExpenseType` | 4 | — | — | no |
| 601 | `schema:IncentiveStatus` | 4 | — | — | no |
| 602 | `schema:PurchaseType` | 4 | — | — | no |
| 603 | `https://agriculture.ld.admin.ch/plant-protection/BacterialBiologicalAgent` | 4 | — | — | no |
| 604 | `https://agriculture.ld.admin.ch/plant-protection/Nematicide` | 4 | — | — | no |
| 605 | `https://environment.ld.admin.ch/foen/nfi/UnitDim` | 3 | — | — | no |
| 606 | `https://environment.ld.admin.ch/foen/nfi/NumericFormat` | 3 | — | — | no |
| 607 | `schema:GamePlayMode` | 3 | — | — | no |
| 608 | `schema:DigitalDocumentPermissionType` | 3 | — | — | no |
| 609 | `schema:CarUsageType` | 3 | — | — | no |
| 610 | `schema:MedicalEvidenceLevel` | 3 | — | — | no |
| 611 | `schema:RefundTypeEnumeration` | 3 | — | — | no |
| 612 | `schema:LegalForceStatus` | 3 | — | — | no |
| 613 | `schema:ItemListOrderType` | 3 | — | — | no |
| 614 | `schema:DeliveryMethod` | 3 | — | — | no |
| 615 | `schema:EventAttendanceModeEnumeration` | 3 | — | — | no |
| 616 | `schema:DrugCostCategory` | 3 | — | — | no |
| 617 | `schema:RsvpResponseType` | 3 | — | — | no |
| 618 | `schema:ReturnLabelSourceEnumeration` | 3 | — | — | no |
| 619 | `rico:Rule` | 3 | — | — | no |
| 620 | `https://agriculture.ld.admin.ch/system-map/CantonalVeterinaryService` | 3 | — | — | no |
| 621 | `qudt:DigitalCurrencyUnit` | 3 | — | — | no |
| 622 | `cube:SharedDimension` | 3 | — | — | no |
| 623 | `http://www.w3.org/ns/org#Organization` | 3 | — | — | no |
| 624 | `https://agriculture.ld.admin.ch/eCH-0265/2/NutrientBalanceCropCategory` | 3 | — | — | no |
| 625 | `rico:DocumentaryFormType` | 2 | — | — | no |
| 626 | `schema:GenderType` | 2 | — | — | no |
| 627 | `https://ch.paf.link/ProceduralRequestType` | 2 | — | — | no |
| 628 | `https://environment.ld.admin.ch/foen/nfi/InventoryType` | 2 | — | — | no |
| 629 | `https://environment.ld.admin.ch/foen/nfi/EvaluationType` | 2 | — | — | no |
| 630 | `https://environment.ld.admin.ch/foen/nfi/StandardError` | 2 | — | — | no |
| 631 | `qudt:AspectClass` | 2 | — | — | no |
| 632 | `schema:MedicalAudienceType` | 2 | — | — | no |
| 633 | `schema:MedicalDevicePurpose` | 2 | — | — | no |
| 634 | `schema:Boolean` | 2 | — | — | no |
| 635 | `schema:BoardingPolicyType` | 2 | — | — | no |
| 636 | `schema:ContactPointOption` | 2 | — | — | no |
| 637 | `schema:SteeringPositionValue` | 2 | — | — | no |
| 638 | `schema:MedicalProcedureType` | 2 | — | — | no |
| 639 | `schema:NLNonprofitType` | 2 | — | — | no |
| 640 | `schema:DrugPrescriptionStatus` | 2 | — | — | no |
| 641 | `owl:DeprecatedClass` | 2 | — | — | no |
| 642 | `owl:DeprecatedProperty` | 2 | — | — | no |
| 643 | `schema:SizeSystemEnumeration` | 2 | — | — | no |
| 644 | `schema:GameAvailabilityEnumeration` | 2 | — | — | no |
| 645 | `schema:CertificationStatusEnumeration` | 2 | — | — | no |
| 646 | `https://register.ld.admin.ch/termdat/52453` | 2 | — | — | no |
| 647 | `https://agriculture.ld.admin.ch/plant-protection/Viricide` | 2 | — | — | no |
| 648 | `https://agriculture.ld.admin.ch/plant-protection/SignalWord` | 2 | — | — | no |
| 649 | `https://environment.ld.admin.ch/foen/wood-processing-survey/1/RegionType` | 2 | — | — | no |
| 650 | `shdim:Entrypoint` | 1 | — | — | no |
| 651 | `shdim:Hierarchies` | 1 | — | — | no |
| 652 | `shdim:SharedDimensionTerms` | 1 | — | — | no |
| 653 | `shdim:SharedDimensionExport` | 1 | — | — | no |
| 654 | `shdim:SharedDimensions` | 1 | — | — | no |
| 655 | `shdim:HierarchyProxy` | 1 | — | — | no |
| 656 | `http://purl.org/vocommons/voaf#Vocabulary` | 1 | — | — | no |
| 657 | `http://www.w3.org/2006/http#GetRequest` | 1 | — | — | no |
| 658 | `http://www.w3.org/2006/http#Response` | 1 | — | — | no |
| 659 | `owl:FunctionalDataProperty` | 1 | — | — | no |
| 660 | `https://environment.ld.admin.ch/foen/nfi/InventoryRegion` | 1 | — | — | no |
| 661 | `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit` | 1 | — | — | no |
| 662 | `rdfs:Property` | 1 | — | — | no |
| 663 | `schema2:Consortium` | 1 | — | — | no |
| 664 | `schema:NGO` | 1 | — | — | no |
| 665 | `schema:EnergyStarEnergyEfficiencyEnumeration` | 1 | — | — | no |
| 666 | `http://www.linkedmodel.org/schema/vaem#Party` | 1 | — | — | no |
| 667 | `qudt:SymmetricRelation` | 1 | — | — | no |
| 668 | `owl:InverseFunctionalProperty` | 1 | — | — | no |
| 669 | `qudt:ParameterModifiabilityType` | 1 | — | — | no |
| 670 | `schema:MeasurementMethodEnum` | 1 | — | — | no |
| 671 | `rico:ActivityType` | 1 | — | — | no |
| 672 | `https://agriculture.ld.admin.ch/system-map/CantonalAgriculturalAdvisor` | 1 | — | — | no |
| 673 | `https://described.at/spex/DefaultShapes` | 1 | — | — | no |
| 674 | `schema:Organisation` | 1 | — | — | no |
| 675 | `https://agriculture.ld.admin.ch/eCH-0265/2/DirectPaymentCropScheme` | 1 | — | — | no |
| 676 | `https://agriculture.ld.admin.ch/eCH-0265/2/NutrientBalanceCropScheme` | 1 | — | — | no |
| 677 | `https://agriculture.ld.admin.ch/eCH-0265/2/PlantProtectionCropScheme` | 1 | — | — | no |

## Gaps — classes over the threshold with no template

### Missing a class template (167)

| Class | Instances | Instance tpl |
|---|---:|:---:|
| `rico:DateRange` | 5,081,934 | — |
| `rico:RecordSet` | 4,818,811 | — |
| `schema:PropertyValue` | 2,412,516 | — |
| `schema:PostalAddress` | 828,573 | — |
| `locn:Address` | 792,332 | — |
| `https://lod.opentransportdata.swiss/vocab/Relation` | 577,343 | — |
| `https://www.w3.org/ns/oa#Annotation` | 397,886 | — |
| `vl:Version` | 366,253 | — |
| `rico:Record` | 263,123 | — |
| `vl:Identity` | 158,435 | — |
| `https://lod.opentransportdata.swiss/vocab/TransportEdge` | 156,985 | — |
| `schch:Synonym` | 63,139 | — |
| `geo:Geometry` | 54,341 | — |
| `schema:CivicStructure` | 54,115 | — |
| `gtfs:Station` | 54,115 | — |
| `rico:Instantiation` | 41,976 | — |
| `schema:Offer` | 40,291 | — |
| `vl:Deprecated` | 37,502 | — |
| `schch:ValidatedEntry` | 35,249 | — |
| `https://lod.opentransportdata.swiss/vocab/ZoningPriceCharacteristic` | 23,859 | — |
| `schema:QuantitativeValue` | 21,933 | — |
| `foaf:Agent` | 20,996 | — |
| `foaf:Organization` | 20,427 | — |
| `rico:Activity` | 17,657 | — |
| `schch:Name` | 16,420 | — |
| `schema:GovernmentOrganization` | 12,292 | — |
| `http://www.w3.org/2006/time#Instant` | 11,211 | — |
| `schch:Abbreviation` | 8,918 | — |
| `schema:Role` | 8,627 | — |
| `rico:RecordResourceToRecordResourceRelation` | 8,598 | — |
| `rdf:List` | 8,130 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Indication` | 7,878 | — |
| `cube:KeyDimension` | 6,745 | — |
| `http://www.w3.org/2006/time#Interval` | 6,313 | — |
| `https://lod.opentransportdata.swiss/vocab/Tarifwertpreisauspraegung` | 6,131 | — |
| `vl:ChangeEvent` | 6,099 | — |
| `https://ld.admin.ch/ech/71/MunicipalityVersion` | 5,868 | — |
| `schch:MunicipalityChangeEvent` | 5,845 | — |
| `http://www.w3.org/2006/time#ProperInterval` | 5,765 | — |
| `qudt:FactorUnit` | 4,871 | — |
| `https://lod.opentransportdata.swiss/vocab/PayLevel` | 4,735 | — |
| `schch:Phraseology` | 4,394 | — |
| `schema:CreativeWork` | 4,230 | — |
| `schch:InProgressEntry` | 4,195 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Ingredient` | 4,078 | — |
| `cube:MeasureDimension` | 4,057 | — |
| `https://agriculture.ld.admin.ch/inspection/InspectionPoint` | 3,952 | — |
| `schema:OrganizationRole` | 3,569 | — |
| `https://ld.admin.ch/ech/71/Municipality` | 3,454 | — |
| `https://lod.opentransportdata.swiss/vocab/Relationsgebiet` | 3,454 | — |
| `vl:InitialRecording` | 3,340 | — |
| `schch:PoliticalMunicipality` | 3,250 | — |
| `http://purl.org/linked-data/cube#Observation` | 3,229 | — |
| `https://lod.opentransportdata.swiss/vocab/Zone` | 3,188 | — |
| `qudt:Unit` | 2,992 | — |
| `https://ld.admin.ch/ech/71/MunicipalityChangeEvent` | 2,703 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Product` | 2,384 | — |
| `http://www.w3.org/ns/prov#Association` | 2,373 | — |
| `dcat:Dataset` | 2,126 | — |
| `schch:Isil` | 2,118 | — |
| `sh:NodeShape` | 2,083 | — |
| `vcard:Organization` | 2,029 | — |
| `schema:ContactPoint` | 1,992 | — |
| `void:Dataset` | 1,983 | — |
| `schema:LobbyOrganization` | 1,982 | — |
| `schema:Dataset` | 1,943 | — |
| `dct:Standard` | 1,901 | — |
| `dct:MediaType` | 1,901 | — |
| `http://rdf-vocabulary.ddialliance.org/xkos#ClassificationLevel` | 1,646 | — |
| `owl:NamedIndividual` | 1,638 | — |
| `https://ch.paf.link/ProceduralRequestInformationActivity` | 1,604 | — |
| `http://www.w3.org/2006/time#GeneralDateTimeDescription` | 1,515 | — |
| `vl:ChangeInHierarchy` | 1,498 | — |
| `https://agriculture.ld.admin.ch/foag/Product` | 1,485 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Obligation` | 1,476 | — |
| `https://ch.paf.link/ParliamentaryAffairIdentifierEntity` | 1,234 | — |
| `https://ch.paf.link/ProceduralRequestEntity` | 1,234 | — |
| `qudt:QuantityKind` | 1,223 | — |
| `https://ch.paf.link/ProceduralRequestInformationEntity` | 1,174 | — |
| `https://agriculture.ld.admin.ch/plant-protection/RegularProduct` | 1,120 | — |
| `relation:StandardError` | 1,026 | — |
| `https://agriculture.ld.admin.ch/plant-protection/ApplicationComment` | 943 | — |
| `dct:PeriodOfTime` | 887 | — |
| `schema:GeoShape` | 842 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Herbicide` | 840 | — |
| `https://lod.opentransportdata.swiss/vocab/Tarif` | 837 | — |
| `https://ch.paf.link/ProceduralRequestProposalActivity` | 769 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/CultivationType` | 755 | — |
| `dct:Collection` | 716 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Fungicide` | 688 | — |
| `owl:ObjectProperty` | 671 | — |
| `https://agriculture.ld.admin.ch/plant-protection/ParallelImport` | 643 | — |
| `https://ch.paf.link/ProceduralRequestProposalEntity` | 635 | — |
| `https://agriculture.ld.admin.ch/plant-protection/SalePermission` | 621 | — |
| `owl:Restriction` | 603 | — |
| `https://ch.paf.link/ProceduralRequestConnex` | 564 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Pest` | 527 | — |
| `http://www.w3.org/ns/hydra/core#Resource` | 513 | — |
| `dcat:Relationship` | 455 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Substance` | 453 | — |
| `qudt:DerivedUnit` | 393 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Insecticide` | 369 | — |
| `qudt:CurrencyUnit` | 360 | — |
| `euvoc:Country` | 345 | — |
| `https://agriculture.ld.admin.ch/plant-protection/ActiveSubstance` | 339 | — |
| `https://lod.opentransportdata.swiss/vocab/Preistabelle` | 334 | — |
| `qudt:PhysicalConstant` | 331 | — |
| `qudt:ConstantValue` | 331 | — |
| `schema:AdministrativeArea` | 327 | — |
| `schema:Event` | 326 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/PlantProtectionCrop` | 326 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Crop` | 326 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/NutrientBalanceCrop` | 314 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Acaricide` | 312 | — |
| `https://ld.admin.ch/ech/71/DistrictVersion` | 309 | — |
| `https://ld.admin.ch/ech/71/District` | 265 | — |
| `schch:DistrictChangeEvent` | 254 | — |
| `qudt:QuantityKindDimensionVector` | 247 | — |
| `https://agriculture.ld.admin.ch/foag/ProductSubgroup` | 246 | — |
| `http://example.com/HydroMeasuringStation` | 233 | — |
| `euvoc:FileType` | 228 | — |
| `schch:TermSubDomain` | 226 | — |
| `qudt:QuantityKindDimensionVector_SI` | 225 | — |
| `qudt:QuantityKindDimensionVector_ISO` | 221 | — |
| `qudt:QuantityKindDimensionVector_Imperial` | 221 | — |
| `owl:DatatypeProperty` | 214 | — |
| `schema:ParliamentaryCommittee` | 186 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/DirectPaymentCrop` | 182 | — |
| `schch:Session` | 181 | — |
| `https://lod.opentransportdata.swiss/vocab/Anwendungsbereich` | 165 | — |
| `schema:BodyOfWater` | 162 | — |
| `sh:PropertyShape` | 160 | — |
| `owl:AnnotationProperty` | 154 | — |
| `https://lod.opentransportdata.swiss/vocab/LocalNetwork` | 150 | — |
| `schema:SoftwareApplication` | 150 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-B1CA8D76547D0FCC5B64F8509C69302C` | 150 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-7DB9868683ED71F8CB39ADD5913D6BAD` | 150 | — |
| `https://agriculture.ld.admin.ch/plant-protection/PlantGrowthRegulator` | 145 | — |
| `https://environment.ld.admin.ch/foen/gefahren-waldbrand/Region` | 143 | — |
| `http://www.w3.org/2011/content#ContentAsText` | 142 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-D0DEFE4EB3BFF0EB57AB219BF1C6058E` | 121 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-A6C91639A6E3E4BC8E83684D78CF9710` | 121 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-C6DCFE0BE13BDD1C4A7AA77B25F6E1AA` | 121 | — |
| `https://agriculture.ld.admin.ch/plant-protection/BeneficialInsectAgent` | 121 | — |
| `schch:TerminologyCollection` | 107 | — |
| `owl:FunctionalProperty` | 104 | — |
| `qudt:QuantityKindDimensionVector_CGS` | 101 | — |
| `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/2777` | 95 | — |
| `schema:webpage` | 95 | — |
| `schch:ChemicalElement` | 92 | — |
| `schema:DigitalDocument` | 91 | — |
| `schema:PoliticalParty` | 82 | — |
| `https://lod.opentransportdata.swiss/vocab/ZoningPlan` | 82 | — |
| `vl:ChangeOfName` | 80 | — |
| `https://agriculture.ld.admin.ch/plant-protection/GHSLabelElement` | 74 | — |
| `https://agriculture.ld.admin.ch/system-map/PrivateOrganization` | 72 | — |
| `qudt:ContextualUnit` | 71 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Molluscicide` | 68 | — |
| `https://agriculture.ld.admin.ch/foag/DataSource` | 66 | — |
| `https://agriculture.ld.admin.ch/plant-protection/HazardStatement` | 64 | — |
| `cube:AttributeDimension` | 64 | — |
| `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2207` | 58 | — |
| `https://ld.admin.ch/ech/71/DistrictChangeEvent` | 58 | — |
| `https://agriculture.ld.admin.ch/system-map/CantonalOrganization` | 51 | — |
| `https://environment.ld.admin.ch/foen/nfi/Unit` | 50 | — |
| `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1030` | 50 | — |
| `qudt:CountingUnit` | 50 | — |

### Missing an instance template (171)

| Class | Instances | Class tpl |
|---|---:|:---:|
| `rico:DateRange` | 5,081,934 | — |
| `rico:RecordSet` | 4,818,811 | — |
| `schema:PropertyValue` | 2,412,516 | — |
| `schema:PostalAddress` | 828,573 | — |
| `schema:Organization` | 818,880 | ✅ |
| `locn:Address` | 792,332 | — |
| `https://lod.opentransportdata.swiss/vocab/Relation` | 577,343 | — |
| `https://www.w3.org/ns/oa#Annotation` | 397,886 | — |
| `vl:Version` | 366,253 | — |
| `rico:Record` | 263,123 | — |
| `vl:Identity` | 158,435 | — |
| `https://lod.opentransportdata.swiss/vocab/TransportEdge` | 156,985 | — |
| `schch:Synonym` | 63,139 | — |
| `geo:Geometry` | 54,341 | — |
| `schema:CivicStructure` | 54,115 | — |
| `gtfs:Station` | 54,115 | — |
| `rico:Instantiation` | 41,976 | — |
| `schema:Offer` | 40,291 | — |
| `vl:Deprecated` | 37,502 | — |
| `schch:ValidatedEntry` | 35,249 | — |
| `https://lod.opentransportdata.swiss/vocab/ZoningPriceCharacteristic` | 23,859 | — |
| `schema:QuantitativeValue` | 21,933 | — |
| `foaf:Agent` | 20,996 | — |
| `foaf:Organization` | 20,427 | — |
| `rico:Activity` | 17,657 | — |
| `schch:Name` | 16,420 | — |
| `schema:GovernmentOrganization` | 12,292 | — |
| `schema:Person` | 12,291 | ✅ |
| `http://www.w3.org/2006/time#Instant` | 11,211 | — |
| `schch:Abbreviation` | 8,918 | — |
| `schema:Role` | 8,627 | — |
| `rico:RecordResourceToRecordResourceRelation` | 8,598 | — |
| `rdf:List` | 8,130 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Indication` | 7,878 | — |
| `cube:KeyDimension` | 6,745 | — |
| `http://www.w3.org/2006/time#Interval` | 6,313 | — |
| `https://lod.opentransportdata.swiss/vocab/Tarifwertpreisauspraegung` | 6,131 | — |
| `vl:ChangeEvent` | 6,099 | — |
| `https://ld.admin.ch/ech/71/MunicipalityVersion` | 5,868 | — |
| `schch:MunicipalityChangeEvent` | 5,845 | — |
| `http://www.w3.org/2006/time#ProperInterval` | 5,765 | — |
| `qudt:FactorUnit` | 4,871 | — |
| `https://lod.opentransportdata.swiss/vocab/PayLevel` | 4,735 | — |
| `schch:Phraseology` | 4,394 | — |
| `schema:CreativeWork` | 4,230 | — |
| `schch:InProgressEntry` | 4,195 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Ingredient` | 4,078 | — |
| `cube:MeasureDimension` | 4,057 | — |
| `https://agriculture.ld.admin.ch/inspection/InspectionPoint` | 3,952 | — |
| `schema:OrganizationRole` | 3,569 | — |
| `https://ld.admin.ch/ech/71/Municipality` | 3,454 | — |
| `https://lod.opentransportdata.swiss/vocab/Relationsgebiet` | 3,454 | — |
| `vl:InitialRecording` | 3,340 | — |
| `schch:PoliticalMunicipality` | 3,250 | — |
| `http://purl.org/linked-data/cube#Observation` | 3,229 | — |
| `https://lod.opentransportdata.swiss/vocab/Zone` | 3,188 | — |
| `qudt:Unit` | 2,992 | — |
| `https://ld.admin.ch/ech/71/MunicipalityChangeEvent` | 2,703 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Product` | 2,384 | — |
| `http://www.w3.org/ns/prov#Association` | 2,373 | — |
| `dcat:Dataset` | 2,126 | — |
| `schch:Isil` | 2,118 | — |
| `sh:NodeShape` | 2,083 | — |
| `rdf:Property` | 2,076 | ✅ |
| `vcard:Organization` | 2,029 | — |
| `schema:ContactPoint` | 1,992 | — |
| `void:Dataset` | 1,983 | — |
| `schema:LobbyOrganization` | 1,982 | — |
| `schema:Dataset` | 1,943 | — |
| `dct:Standard` | 1,901 | — |
| `dct:MediaType` | 1,901 | — |
| `http://rdf-vocabulary.ddialliance.org/xkos#ClassificationLevel` | 1,646 | — |
| `owl:NamedIndividual` | 1,638 | — |
| `https://ch.paf.link/ProceduralRequestInformationActivity` | 1,604 | — |
| `http://www.w3.org/2006/time#GeneralDateTimeDescription` | 1,515 | — |
| `vl:ChangeInHierarchy` | 1,498 | — |
| `https://agriculture.ld.admin.ch/foag/Product` | 1,485 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Obligation` | 1,476 | — |
| `owl:Class` | 1,415 | ✅ |
| `https://ch.paf.link/ParliamentaryAffairIdentifierEntity` | 1,234 | — |
| `https://ch.paf.link/ProceduralRequestEntity` | 1,234 | — |
| `qudt:QuantityKind` | 1,223 | — |
| `https://ch.paf.link/ProceduralRequestInformationEntity` | 1,174 | — |
| `https://agriculture.ld.admin.ch/plant-protection/RegularProduct` | 1,120 | — |
| `relation:StandardError` | 1,026 | — |
| `https://agriculture.ld.admin.ch/plant-protection/ApplicationComment` | 943 | — |
| `dct:PeriodOfTime` | 887 | — |
| `schema:GeoShape` | 842 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Herbicide` | 840 | — |
| `https://lod.opentransportdata.swiss/vocab/Tarif` | 837 | — |
| `https://ch.paf.link/ProceduralRequestProposalActivity` | 769 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/CultivationType` | 755 | — |
| `dct:Collection` | 716 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Fungicide` | 688 | — |
| `owl:ObjectProperty` | 671 | — |
| `https://agriculture.ld.admin.ch/plant-protection/ParallelImport` | 643 | — |
| `https://ch.paf.link/ProceduralRequestProposalEntity` | 635 | — |
| `https://agriculture.ld.admin.ch/plant-protection/SalePermission` | 621 | — |
| `owl:Restriction` | 603 | — |
| `https://ch.paf.link/ProceduralRequestConnex` | 564 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Pest` | 527 | — |
| `http://www.w3.org/ns/hydra/core#Resource` | 513 | — |
| `dcat:Relationship` | 455 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Substance` | 453 | — |
| `qudt:DerivedUnit` | 393 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Insecticide` | 369 | — |
| `qudt:CurrencyUnit` | 360 | — |
| `euvoc:Country` | 345 | — |
| `https://agriculture.ld.admin.ch/plant-protection/ActiveSubstance` | 339 | — |
| `https://lod.opentransportdata.swiss/vocab/Preistabelle` | 334 | — |
| `qudt:PhysicalConstant` | 331 | — |
| `qudt:ConstantValue` | 331 | — |
| `schema:AdministrativeArea` | 327 | — |
| `schema:Event` | 326 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/PlantProtectionCrop` | 326 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Crop` | 326 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/NutrientBalanceCrop` | 314 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Acaricide` | 312 | — |
| `https://ld.admin.ch/ech/71/DistrictVersion` | 309 | — |
| `https://ld.admin.ch/ech/71/District` | 265 | — |
| `schch:DistrictChangeEvent` | 254 | — |
| `qudt:QuantityKindDimensionVector` | 247 | — |
| `https://agriculture.ld.admin.ch/foag/ProductSubgroup` | 246 | — |
| `http://example.com/HydroMeasuringStation` | 233 | — |
| `euvoc:FileType` | 228 | — |
| `schch:TermSubDomain` | 226 | — |
| `qudt:QuantityKindDimensionVector_SI` | 225 | — |
| `qudt:QuantityKindDimensionVector_ISO` | 221 | — |
| `qudt:QuantityKindDimensionVector_Imperial` | 221 | — |
| `owl:DatatypeProperty` | 214 | — |
| `schema:ParliamentaryCommittee` | 186 | — |
| `https://agriculture.ld.admin.ch/eCH-0265/2/DirectPaymentCrop` | 182 | — |
| `schch:Session` | 181 | — |
| `https://lod.opentransportdata.swiss/vocab/Anwendungsbereich` | 165 | — |
| `schema:BodyOfWater` | 162 | — |
| `sh:PropertyShape` | 160 | — |
| `owl:AnnotationProperty` | 154 | — |
| `https://lod.opentransportdata.swiss/vocab/LocalNetwork` | 150 | — |
| `schema:SoftwareApplication` | 150 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-B1CA8D76547D0FCC5B64F8509C69302C` | 150 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-7DB9868683ED71F8CB39ADD5913D6BAD` | 150 | — |
| `https://agriculture.ld.admin.ch/plant-protection/PlantGrowthRegulator` | 145 | — |
| `https://environment.ld.admin.ch/foen/gefahren-waldbrand/Region` | 143 | — |
| `http://www.w3.org/2011/content#ContentAsText` | 142 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-D0DEFE4EB3BFF0EB57AB219BF1C6058E` | 121 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-A6C91639A6E3E4BC8E83684D78CF9710` | 121 | — |
| `genid-b6e190ffca0942d28032a27ec4f915391681662-C6DCFE0BE13BDD1C4A7AA77B25F6E1AA` | 121 | — |
| `https://agriculture.ld.admin.ch/plant-protection/BeneficialInsectAgent` | 121 | — |
| `schch:TerminologyCollection` | 107 | — |
| `owl:FunctionalProperty` | 104 | — |
| `qudt:QuantityKindDimensionVector_CGS` | 101 | — |
| `https://environment.ld.admin.ch/foen/nfi/UnitOfReference/2777` | 95 | — |
| `schema:webpage` | 95 | — |
| `schch:ChemicalElement` | 92 | — |
| `schema:DigitalDocument` | 91 | — |
| `schema:PoliticalParty` | 82 | — |
| `https://lod.opentransportdata.swiss/vocab/ZoningPlan` | 82 | — |
| `vl:ChangeOfName` | 80 | — |
| `https://agriculture.ld.admin.ch/plant-protection/GHSLabelElement` | 74 | — |
| `https://agriculture.ld.admin.ch/system-map/PrivateOrganization` | 72 | — |
| `qudt:ContextualUnit` | 71 | — |
| `https://agriculture.ld.admin.ch/plant-protection/Molluscicide` | 68 | — |
| `https://agriculture.ld.admin.ch/foag/DataSource` | 66 | — |
| `https://agriculture.ld.admin.ch/plant-protection/HazardStatement` | 64 | — |
| `cube:AttributeDimension` | 64 | — |
| `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/2207` | 58 | — |
| `https://ld.admin.ch/ech/71/DistrictChangeEvent` | 58 | — |
| `https://agriculture.ld.admin.ch/system-map/CantonalOrganization` | 51 | — |
| `https://environment.ld.admin.ch/foen/nfi/Unit` | 50 | — |
| `https://environment.ld.admin.ch/foen/nfi/ClassificationUnit/1030` | 50 | — |
| `qudt:CountingUnit` | 50 | — |
