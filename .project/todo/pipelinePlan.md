# Data Structure Harvesting Pipeline — Requirements

**Status:** Draft (2026-09-05) — M1 implemented
**Component:** new — `cmd/visoto-harvest/`, `internal/pipeline/*`
**Type:** feature / new component
**Depends on:** `internal/config`, `internal/sparql`, a writable QLever endpoint

## 1. Purpose

Derive the *structure* of open-government datasets from the actual data, not from
the (usually absent) documentation, and publish that structure as RDF next to the
DCAT metadata it belongs to.

A portal such as opendata.swiss tells you a distribution is a CSV called
`bevoelkerung_2024.csv`, 2.3 MB, licence CC-BY. It does not tell you the file has
14 columns, that column 3 is a four-digit integer that looks like a BFS municipality
number, or that the same column appears in 47 other datasets. The pipeline recovers
that by downloading the file and reading it — reverse engineering the schema — and
writes the result into the triplestore where Visoto can already render and query it.

Two phases, deliberately separated:

1. **Observation** — what is measurably in the file (columns, types, cardinalities,
   XML element paths, RDF class/property partitions). Deterministic, reproducible.
2. **Interpretation** — what the observed structure *means* (datatypes refined,
   semantic types, links to known registers). Heuristic, confidence-bearing,
   never overwrites phase 1.

## 2. Goals

- **G1** Harvest DCAT catalogues from multiple, pluggable sources and load the
  metadata itself into the triplestore.
- **G2** Download distributions and extract their structure without human input.
- **G3** Model that structure as RDF hanging directly off the harvested
  `dcat:Distribution` IRIs, so metadata and structure are one graph.
- **G4** Make structure comparable *across* datasets and publishers (field
  signatures, §6.4) so join candidates and reused code lists become queryable.
- **G5** Re-run cheaply and detect structural change over time.
- **G6** Later: classify fields by datatype and semantic meaning, with provenance
  and confidence.

## 3. Non-goals

- Not a data warehouse. Cell values are never bulk-loaded; only aggregates and
  short samples.
- Not a catalogue replacement. The source portals stay authoritative for metadata;
  we mirror, we do not curate.
- Not a validation service (though the emitted SHACL projection makes one possible
  later).
- No write path from the Visoto web UI into the pipeline in v1. The pipeline is a
  batch job; the web app is a read-only consumer.

## 4. Glossary

| Term | Meaning |
|---|---|
| **Source** | A catalogue a `Source` adapter can harvest (opendata.swiss, data.europa.eu, …) |
| **Dataset / Distribution** | DCAT senses. A distribution is one downloadable file or API. |
| **Blob** | The downloaded bytes of one distribution, content-addressed by SHA-256. |
| **Structure** | The derived schema of one blob: fields, cardinalities, statistics. |
| **Field** | One addressable position inside a structure — a CSV column, an XML element path, a JSON pointer, an RDF predicate. |
| **Signature** | A content-derived identity shared by fields that hold the same kind of values, across distributions (§6.4). |
| **Run** | One execution of the pipeline over a source; the unit of provenance and of graph versioning. |

## 5. Architecture

### 5.1 Stages

```
  ┌─────────┐   ┌───────┐   ┌───────┐   ┌─────────┐   ┌──────┐   ┌──────┐
  │ discover│──▶│ fetch │──▶│ sniff │──▶│ profile │──▶│ mint │──▶│ load │
  └─────────┘   └───────┘   └───────┘   └─────────┘   └──────┘   └──────┘
       │                                                             │
       │  DCAT-AP quads ────────────────────────────────────────────▶│
       │                                                             ▼
       └── state ──▶ SQLite (queue, etags, hashes, sketches)     QLever
                                                                     │
                                                        ┌────────────┘
                                                        ▼
                                                  ┌──────────┐
                                                  │ classify │  (phase 2)
                                                  └──────────┘
```

Each stage is a pure function of its input plus the state row, so any stage can be
re-run for a single distribution without re-running the ones before it.

- **R-ARCH-1** The pipeline ships as its own binary, `cmd/visoto-harvest`, with no
  runtime dependency on the web server. Both read the same `visoto.config`.
- **R-ARCH-2** Stage transitions are recorded in SQLite before the side effect, so
  an interrupted run resumes without repeating downloads.
- **R-ARCH-3** The pipeline is restartable at any point and idempotent: two runs
  over an unchanged catalogue produce identical triples (modulo run provenance).
- **R-ARCH-4** All three extension points — sources, profilers, loaders — use the
  registry + fallback pattern already established in `internal/export/provider.go`.
- **R-ARCH-5** Following the project convention, stage logic is expressed as
  methods on stage structs rather than free functions.

### 5.2 Modular sources

The single most important interface. **Source adapters emit DCAT-AP, never their
native shape.** Everything downstream sees one vocabulary, whatever the portal
speaks — SPARQL, CKAN JSON, an RDF dump, or a future adapter for an internal
inventory.

```go
// Source harvests a catalogue and emits normalized DCAT-AP records.
type Source interface {
    Name() string                      // stable slug, e.g. "opendata-swiss"
    Harvest(ctx context.Context, since time.Time, emit EmitFunc) error
}

type EmitFunc func(Record) error

// Record is one dcat:Dataset with its distributions, already in DCAT-AP.
type Record struct {
    DatasetIRI    string
    Quads         []Quad         // verbatim DCAT-AP for the catalog graph
    Distributions []Distribution // fetch targets extracted from those quads
}

type Distribution struct {
    IRI           string
    DownloadURL   string
    DeclaredMedia string   // dcat:mediaType — advisory only, frequently wrong
    ByteSize      int64    // advisory
    Licence       string
    Modified      time.Time
}
```

- **R-SRC-1** At least two adapters in v1: `dcat-sparql` (data.europa.eu, and
  opendata.swiss where it exposes DCAT) and `ckan` (opendata.swiss CKAN API).
- **R-SRC-2** Adding a source requires no change outside its own package and one
  `RegisterSource` call.
- **R-SRC-3** Adapters support incremental harvest via a `since` watermark
  (`dct:modified`), persisted per source. A full re-harvest is an explicit flag.
- **R-SRC-4** The `ckan` adapter maps CKAN JSON into DCAT-AP. Where a mapping is
  lossy, unmapped fields are dropped, not invented.
- **R-SRC-5** Adapters must be able to run against a fixture directory for tests —
  no network in unit tests.
- **R-SRC-6** Per-source configuration (URL, credentials, page size, rate limit,
  theme filter) lives in `[[pipeline.sources]]`, §9.

### 5.3 Catalogue ingestion (G1)

The harvested DCAT is loaded into the triplestore, not just used as a worklist.
This is what lets a single query walk publisher → dataset → distribution → column
→ semantic type.

- **R-CAT-1** DCAT-AP quads are written verbatim into a catalogue graph per source
  and run: `<…/graph/catalog/{source}/{runID}>`.
- **R-CAT-2** Structure graphs reference the *same* `dcat:Distribution` IRIs the
  catalogue uses. No parallel identifier space for things the portal already names.
- **R-CAT-3** Where a source exposes blank nodes (CKAN-derived distributions,
  contact points), they are skolemized deterministically from the parent IRI plus
  a stable local key, so re-runs produce the same IRIs.
- **R-CAT-4** Catalogue graphs are additive per run; a retention policy (§8) prunes
  old runs. The most recent run per source is discoverable via a small pointer
  graph (`viso:currentCatalogGraph`) rather than by date arithmetic in every query.

### 5.4 Fetch

- **R-FET-1** Content-addressed store: blobs land at `work/blobs/{sha256[0:2]}/{sha256}`.
- **R-FET-2** Conditional requests (`If-None-Match`, `If-Modified-Since`) using the
  values stored from the previous fetch. A 304 skips straight to `load` as a no-op.
- **R-FET-3** An unchanged SHA-256 skips `profile` even when the server sends no
  validators.
- **R-FET-4** Hard size cap (`max_download_bytes`, default 512 MB). Over the cap,
  the distribution is recorded as `viso:skipped` with a reason, never silently lost.
- **R-FET-5** Per-host concurrency limit and minimum request interval; a
  configurable global worker pool. Portal-wide 429/5xx trips a per-host circuit
  breaker that pauses that host for the rest of the run.
- **R-FET-6** Redirects followed to a bounded depth; only `http`/`https`; the
  SSRF guard from `internal/upload` (`validateRemoteURL`) is reused, not
  re-implemented.
- **R-FET-7** A distinguishable `User-Agent` naming the project and a contact URL.

### 5.5 Format detection

- **R-SNF-1** Detection is by content (magic bytes, BOM, first-line probing), with
  the declared `dcat:mediaType` and the URL extension as tie-breakers only.
  Portal-declared media types are wrong often enough to be untrustworthy.
- **R-SNF-2** Both the declared and the detected type are recorded; a mismatch is a
  queryable fact (`viso:declaredMediaType` vs `viso:detectedMediaType`), useful as
  a data-quality signal in its own right.
- **R-SNF-3** Containers (ZIP, GZ, TAR) are expanded and their members treated as
  child distributions (`viso:memberOf`), bounded by member count and total
  uncompressed size to resist zip bombs.

### 5.6 Profiling

```go
// Profiler derives a Structure from one blob of a format it recognizes.
type Profiler interface {
    Name() string
    CanProfile(detectedMedia string, head []byte) bool
    Profile(ctx context.Context, b Blob) (*Structure, error)
}
```

- **R-PRF-1** Profilers are registered and selected by `CanProfile`; an unrecognized
  format yields a `Structure` with format `viso:Unknown` and no fields — never an
  error that stalls the queue.
- **R-PRF-2** Profiling is streaming and memory-bounded. A configurable row/byte
  sampling cap applies; whether a structure was sampled or complete is recorded
  (`viso:profileComplete`, `viso:sampledRows`).
- **R-PRF-3** Per field, the observation set is: name, locator, position, non-null
  count, null count, distinct count (exact under a threshold, HyperLogLog above),
  min/max, min/max length, inferred datatype with the per-candidate hit ratio,
  top-k values with frequencies (k configurable, default 10), and detected value
  patterns.
- **R-PRF-4** Per structure: row/record count, field count, encoding, delimiter and
  quoting for tabular, plus parse-error count and the first few error messages.
- **R-PRF-5** Datatype inference is reported as a *distribution*, not a verdict:
  `xsd:integer 0.98, xsd:string 1.0` says the column is an integer with 2 % dirt,
  which is more useful than either label alone.
- **R-PRF-6** Field signature sketches (§6.4) are computed during this single pass.
  They cannot be added later without re-downloading everything.
- **R-PRF-7** Where an external engine does the work better (DuckDB's
  `DESCRIBE` / `SUMMARIZE` over `read_csv_auto`, `read_json_auto`,
  `read_parquet`), the profiler shells out to it and parses the result. This is an
  implementation choice behind the interface, and must degrade to the built-in
  streaming profiler when the binary is absent.

### 5.7 Minting & loading

- **R-LOD-1** Two loaders behind one interface: `bulk-file` (serialize N-Quads into
  `qlever/data/`, trigger a re-index) and `sparql-update` (`INSERT DATA`, graph
  drop for replacement). Default is `bulk-file`.
- **R-LOD-2** Rationale, from our own operating note in
  `scripts/qlever-start-dev.sh`: heavy SPARQL UPDATE volume degrades QLever query
  performance. Full harvests therefore go through re-index; `sparql-update` is for
  incremental top-ups between re-indexes.
- **R-LOD-3** Loading is transactional per distribution: the new structure graph is
  written and only then the pointer flipped, so a reader never sees a half graph.
- **R-LOD-4** The loader writes nothing when the structure is byte-identical to the
  previous run's structure for that distribution (compare a canonical hash), so an
  unchanged catalogue produces an empty diff.

### 5.8 Classification (phase 2)

- **R-CLS-1** Two tiers, deterministic first. Tier 1 is a registry of validators:
  ISO 8601 dates, ISO country/language codes, IBAN, EAN, coordinates (WGS 84 and
  LV95), Swiss identifiers (BFS municipality number, EGID/EWID, UID, AHV), postal
  codes, e-mail, URL. Cheap, explainable, and it covers most of the Swiss corpus.
- **R-CLS-2** Tier 2 (LLM, via the existing `gemini_api_key`) runs only on fields
  tier 1 could not classify, and is prompted with the field name, its *neighbouring*
  field names, the dataset title and the top-k values. Neighbours matter: `nr` alone
  is unclassifiable, `nr` beside `gemeinde` is not.
- **R-CLS-3** Classification targets *signatures*, not fields (§6.4). Classify once,
  inherit everywhere — the difference between ~10^6 LLM calls and ~10^4.
- **R-CLS-4** Every annotation carries `viso:confidence`, the method
  (`viso:byValidator` / `viso:byModel` / `viso:byHuman`) and `prov:wasGeneratedBy`.
  Annotations are separate nodes; observations from §5.6 are never overwritten.
- **R-CLS-5** Semantic types resolve to real IRIs where one exists — LINDAS
  registers, schema.org, QUDT — because that is what makes the result joinable
  rather than merely descriptive.
- **R-CLS-6** A gold set of ~200 manually labelled fields exists *before* the
  classifier, with precision/recall reported per tier. Without it there is no way
  to tell tier 2 from noise.
- **R-CLS-7** A human correction is representable and outranks both tiers.

### 5.9 Visoto integration

- **R-UI-1** The structure graph is ordinary RDF in an ordinary endpoint. The
  existing class/instance templates and Graph Explorer render it with no pipeline-
  specific code.
- **R-UI-2** Class and instance templates for `viso:Structure`, `viso:Field`,
  `viso:Signature` and `dcat:Distribution` are delivered with the feature
  (`classTemplate` / `instanceTemplate` skills), plus resource icons
  (`iconGeneration`).
- **R-UI-3** The pipeline's QLever instance is registered as a normal
  `[[application.sparqlEndpoints]]` entry with its own `slug` and `tag`, so
  templates can branch on `.EndpointTag`.

### 5.10 Statelessness and scaling out

The question this section answers: can the modularity above be run stateless, and
can it scale beyond one machine?

**Workers can be stateless. The queue cannot.** A worker holds nothing that is not
recoverable, by construction:

| Property | Why it holds | Consequence |
|---|---|---|
| Minted IRIs are content-derived (R-CAT-3, R-SIG-2) | `Minter` hashes inputs, never a counter or a clock | Two workers processing the same distribution produce byte-identical triples — no coordination needed to allocate identity |
| Blobs are content-addressed (R-FET-1) | Path is the SHA-256 | Any worker can find, verify or re-derive a blob |
| Stage transitions are recorded before their side effect (R-ARCH-2) | `SetStage` precedes the work | A crash re-does at most one item |
| Loads are graph-scoped and idempotent (R-LOD-3) | `BeginGraph` + `Append` per graph | A repeated load is a no-op, not a duplicate |

So a worker is disposable: kill it mid-fetch and another produces the same result.
What is irreducibly stateful is the queue — something must record who is working
on what, and what the watermark is.

- **R-SCALE-1** Work is claimed under a **lease**, not a lock: `ClaimBatch`
  hands out rows with an expiry, and a lease that runs out is reclaimed. A lock
  held by a dead process is a stuck queue; a lease is self-healing. An expired
  lease costs duplicate work, never corruption, because of the properties above.
- **R-SCALE-2** The runner takes its job store as an **interface**
  (`harvest.Store`), so the backend is replaceable without touching the stages.
- **R-SCALE-3** The claim is written as SELECT-then-UPDATE inside a transaction.
  Under SQLite the transaction suffices (one writer); the same shape is what
  PostgreSQL implements as `SELECT … FOR UPDATE SKIP LOCKED`. Moving to a shared
  queue therefore replaces one method body, not the pipeline.

**What is single-node today, and what each would cost to lift:**

| Component | Today | To scale out |
|---|---|---|
| Job queue | SQLite, node-local | Swap `harvest.Store` for PostgreSQL; `ClaimBatch` already has the right shape |
| Blob store | Local directory | Object storage behind the same content-addressed key |
| Bulk-file loader | Writes one local file | Shared volume, or per-worker files merged before re-index |
| Rate limiter | In-process, per source | Shared token bucket — otherwise N workers means N× the request rate, and the portal blocks us (R-FET-5) |
| Catalogue harvest | One goroutine per source | Stays that way: paging a catalogue is inherently sequential, and it is not the bottleneck. **Fetch and profile are the parallel stages** |

- **R-SCALE-4** Distributing the fetch stage without first making the rate limiter
  shared is a defect, not an optimization: it converts a scaling change into a
  ban. The limiter is per-source by design so this stays a single seam.
- **R-SCALE-5** Sources are harvested independently and a failing source never
  blocks another (R-NFR-5), so per-source parallelism is available before any of
  the above is done. That is the cheap 80 %: two portals harvest concurrently on
  one machine today.

The honest summary: the design is **horizontally scalable in shape and
single-node in deployment**. Nothing above the job store assumes one machine, and
the two seams that do — the store interface and the limiter — are named here so
they are not discovered late.

## 6. Data model

### 6.1 Namespaces

| Prefix | IRI | Use |
|---|---|---|
| `viso:` | `https://visoto.hutzli.org/ns/structure#` | pipeline vocabulary (terms) |
| — | `https://visoto.hutzli.org/id/` | minted instance IRIs |
| `dcat:`, `dct:`, `prov:`, `sh:`, `csvw:`, `void:`, `xsd:` | standard | reused |

Freeze the base IRI before the first load: rewriting minted IRIs afterwards is the
expensive mistake. Listed again in §11 as an open decision.

### 6.2 Core shape

```turtle
# --- catalogue (harvested verbatim, R-CAT-1) --------------------------------
<https://opendata.swiss/…/dataset/bevoelkerung>
    a dcat:Dataset ;
    dct:title "Bevölkerung nach Gemeinde"@de ;
    dct:publisher <…/organization/bfs> ;
    dcat:distribution <https://opendata.swiss/…/resource/a1b2> .

# --- structure (derived, R-CAT-2: same distribution IRI) --------------------
<https://opendata.swiss/…/resource/a1b2>
    a dcat:Distribution ;
    viso:hasStructure  <https://visoto.hutzli.org/id/structure/a1b2/r7> ;
    viso:contentHash   "sha256:9f3c…" ;
    viso:declaredMediaType "text/csv" ;
    viso:detectedMediaType "application/zip" .   # mismatch is a quality signal

<https://visoto.hutzli.org/id/structure/a1b2/r7>
    a viso:Structure , sh:NodeShape ;
    viso:format          viso:CSV ;
    viso:rowCount        18422 ;
    viso:fieldCount      14 ;
    viso:encoding        "UTF-8" ;
    viso:delimiter       ";" ;
    viso:profileComplete true ;
    viso:parseErrors     0 ;
    prov:wasGeneratedBy  <https://visoto.hutzli.org/id/run/r7> ;
    viso:field           <https://visoto.hutzli.org/id/field/a1b2/gemeinde_bfs> .

<https://visoto.hutzli.org/id/field/a1b2/gemeinde_bfs>
    a viso:Field ;
    viso:locator          "col:3" ;        # XPath / JSON Pointer / predicate IRI per format
    viso:position         3 ;
    viso:name             "gemeinde_bfs" ;
    viso:nonNullCount     18410 ;
    viso:nullCount        12 ;
    viso:distinctCount    2131 ;
    viso:distinctExact    true ;
    viso:min              "1" ; viso:max "6810" ;
    viso:datatypeHit      [ viso:datatype xsd:integer ; viso:ratio 0.998 ] ;
    viso:topValue         [ rdf:value "261" ; viso:frequency 88 ] ;
    viso:signature        <https://visoto.hutzli.org/id/sig/9f3c…> .

# --- interpretation (phase 2, separate nodes, R-CLS-4) ----------------------
<https://visoto.hutzli.org/id/sig/9f3c…>
    a viso:Signature ;
    viso:annotation [
        a viso:SemanticAnnotation ;
        viso:semanticType   <https://ld.admin.ch/municipality> ;
        viso:confidence     0.94 ;
        viso:method         viso:byValidator ;
        prov:wasGeneratedBy <https://visoto.hutzli.org/id/run/r9>
    ] .

# --- provenance -------------------------------------------------------------
<https://visoto.hutzli.org/id/run/r7>
    a viso:HarvestRun , prov:Activity ;
    viso:source          "opendata-swiss" ;
    prov:startedAtTime   "2026-09-05T04:00:00Z"^^xsd:dateTime ;
    prov:endedAtTime     "2026-09-05T06:12:44Z"^^xsd:dateTime ;
    viso:distributionsSeen 4210 ; viso:distributionsProfiled 3877 .
```

- **R-MOD-1** One abstraction spans all formats: a **Field is a locator within a
  structure**. `viso:locator` carries the format-specific address — `col:3` for
  CSV, an XPath for XML, a JSON Pointer for JSON, a predicate IRI for RDF. Only the
  locator syntax varies; every statistic above applies unchanged.
- **R-MOD-2** Nested formats (XML, JSON) express containment with
  `viso:parentField`, so a field tree is walkable without parsing locators.
- **R-MOD-3** Standard vocabularies are emitted as *projections* alongside the
  `viso:` spine, not instead of it: `csvw:Table`/`csvw:Column` for tabular,
  `void:classPartition` / `void:propertyPartition` / `void:triples` for RDF
  distributions (which is a structural summary for free), and a SHACL view
  (`sh:property`, `sh:path`, `sh:datatype`, `sh:minCount`/`sh:maxCount` derived
  from observed cardinalities) so the derived structure is executable as validation
  later. Statistics stay on the `viso:` nodes — neither CSVW nor SHACL carries them
  without abuse.
- **R-MOD-4** Projections are optional per run (`emit_csvw`, `emit_shacl`,
  `emit_void`), because they roughly double triple count.

### 6.3 Graphs & versioning

| Graph | Contents | Lifecycle |
|---|---|---|
| `…/graph/catalog/{source}/{run}` | verbatim DCAT-AP | pruned by retention |
| `…/graph/structure/{distHash}/{run}` | one distribution's structure | replaced per run |
| `…/graph/signature` | signature nodes + annotations | cumulative |
| `…/graph/run` | run provenance | cumulative |
| `…/graph/current` | pointers to the current graph per distribution/source | rewritten |

- **R-VER-1** Run-scoped structure graphs make structural diffing free: "which
  distributions gained or lost a field since run *n*" is a query, not a
  bookkeeping feature. That diff is a headline capability, not a side effect.
- **R-VER-2** Rollback of a bad run = dropping its graphs.
- **R-VER-3** Retention is configurable: keep the last *N* runs per distribution
  (default 3) plus every run in which the structure actually changed.

### 6.4 Field signatures — what they are and why now

**Decided: in scope from v1** (confirmed 2026-09-05). The sketch is computed in
the profiling pass and persisted from the first run.

**The problem.** After profiling 50 000 distributions you hold roughly a million
field descriptions, each sealed inside its own distribution. Nothing in the graph
says that `bfs_nr` in a BFS population file and `gemeinde_id` in a cantonal energy
file hold values from the same universe. Without that link, the harvest is a pile
of isolated schemas — accurate, and not much more useful than the files themselves.

**The idea.** A signature is a *content-derived identity for a field*, minted as its
own IRI, that many fields across many distributions point at. It is computed from
what the profiler already sees:

| Component | Example |
|---|---|
| normalized name | `gemeinde_bfs`, `bfs_nr`, `BFS-Nr.` → `bfsnr` (case, separators, language suffixes stripped) |
| inferred datatype | `xsd:integer` |
| cardinality class | bucketed distinct count / row count |
| value fingerprint | 128-permutation MinHash over the distinct values |
| pattern class | `^\d{4}$` |

Two levels of identity, because neither alone is enough:

- **Exact signature** — a hash of (normalized name, datatype, pattern class). Cheap,
  gives exact buckets, but misses `bfs_nr` vs `gemeinde_id` entirely.
- **Similarity link** — MinHash Jaccard over the value sets, above a threshold.
  This catches the renamed-but-identical case, and it is the one that finds join
  candidates across publishers who never agreed on a column name.

**What it buys.**

1. *Join discovery.* "Which distributions contain a field whose values overlap this
   municipality register?" becomes a two-triple-pattern query. This is the feature
   that turns a structure catalogue into a data-integration tool.
2. *Classification transfer (R-CLS-3).* Classify the signature once and every field
   pointing at it inherits the annotation. ~10^6 fields collapse to ~10^4
   signatures — the difference between an unaffordable LLM bill and a cheap one.
3. *Code-list detection.* A low-cardinality signature recurring across dozens of
   publishers is, empirically, an undocumented code list worth promoting to SKOS.
4. *Quality signals.* Same signature, different declared datatype across publishers
   → a normalization defect, visible without reading either file.

**Why it must be built in from day one.** The MinHash sketch has to be computed
while the values stream past during profiling. Retrofitting it means re-downloading
and re-profiling the entire corpus. The cost of building it now is one sketch object
per field (128 × 4 bytes) held in SQLite; only the resulting signature IRI ever
reaches the triplestore.

- **R-SIG-1** Sketches are computed in the profiling pass (R-PRF-6) and stored in
  SQLite, never in RDF.
- **R-SIG-2** Signature IRIs are content-derived and stable across runs: the same
  inputs mint the same IRI.
- **R-SIG-3** Similarity linking (`viso:similarTo` with `viso:jaccard`) runs as a
  post-load batch job over the sketch table, not inline per distribution.
- **R-SIG-4** Signature membership and similarity thresholds are configurable and
  recorded on the run, since results are not comparable across threshold changes.
- **R-SIG-5** Identity has two levels, because neither works alone: an **exact
  key** over (normalized name, datatype, pattern class, cardinality class) gives
  precision and costs nothing, and the **MinHash similarity link** gives recall
  across publishers who never agreed on a column name. The exact key alone would
  never connect `bfs_nr` to `gemeinde_id`; the sketch alone would connect every
  small integer column in the corpus.
- **R-SIG-6** Repeated observations of one signature **merge** their sketches
  (MinHash union is element-wise minimum), so a signature accumulates the value
  universe of the concept rather than of whichever file was seen first.
- **R-SIG-7** Sketching costs ~210 ns per value. On a 1M-row, 30-column CSV that
  is ~6 s, which is affordable but not free: sketches are computed over the
  sampled rows (R-PRF-2), not necessarily over every row.

## 7. Non-functional requirements

- **R-NFR-1 Scale.** Design target: 100 000 distributions, ~30 fields each. At
  ~15 triples per field that is ~45 M triples plus catalogue and projections —
  comfortable for QLever. Run history and sample values are what actually grow
  without bound, hence R-VER-3.
- **R-NFR-2 Throughput.** A full opendata.swiss harvest completes overnight on one
  machine. Bounded by politeness (R-FET-5), not by CPU.
- **R-NFR-3 Memory.** Constant per worker regardless of file size; no full-file
  buffering anywhere.
- **R-NFR-4 Disk.** Blob store size is bounded and the eviction policy explicit
  (§11 open decision).
- **R-NFR-5 Resilience.** One malformed file, one hostile server, one profiler
  panic must not stop the run. Every failure is recorded against its distribution
  with a reason, and the queue moves on. Panics in profilers are recovered.
- **R-NFR-6 Observability.** Structured logging via `internal/logger`; a run
  summary (seen / fetched / skipped / failed per stage and per source) written both
  to the log and to the run node in RDF, so the pipeline's own health is queryable
  by SPARQL.
- **R-NFR-7 Security.** SSRF guard reused from `internal/upload`; archive expansion
  bounded; no shelling out with unsanitized paths; credentials only from config,
  never in minted IRIs or logs.
- **R-NFR-8 Legality.** Per-distribution licence from DCAT is carried into the
  structure graph. Sample values are short (top-k, k ≤ 10, truncated) so the graph
  is a description, not a redistribution. Portal terms of service and `robots.txt`
  are respected.
- **R-NFR-9 Testability.** Every profiler has fixture-based unit tests with
  golden-file structures. No network in `go test`.

## 8. Configuration

```toml
[pipeline]
enabled            = true
target_endpoint    = "qlever-local"     # slug from [[application.sparqlEndpoints]]
work_dir           = "./pipeline/work"
state_db           = "./pipeline/state.sqlite"
workers            = 8
max_download_bytes = 536870912          # 512 MB
sample_rows        = 200000
top_k              = 10
loader             = "bulk-file"        # or "sparql-update"
emit_csvw          = true
emit_shacl         = true
emit_void          = true
keep_runs          = 3
base_iri           = "https://visoto.hutzli.org/id/"
user_agent         = "visoto-harvest/1.0 (+https://visoto.hutzli.org)"

[[pipeline.sources]]
name       = "opendata-swiss"
type       = "dcat-sparql"
url        = "https://opendata.swiss/…/sparql"
enabled    = true
rate_limit = "2/s"
page_size  = 500

[[pipeline.sources]]
name       = "data-europa"
type       = "dcat-sparql"
url        = "https://data.europa.eu/sparql"
enabled    = false
rate_limit = "1/s"
```

- **R-CFG-1** The pipeline reads the existing `visoto.config`; its absence or
  `enabled = false` leaves the web app unaffected.
- **R-CFG-2** `visoto.config.example` documents every key, matching the commenting
  style already used there.

## 9. Format roadmap

Ordered by corpus coverage against effort. Only phase 1 is in scope for the first
milestone; the rest are here to make sure the interfaces do not have to change to
accommodate them.

| Phase | Formats | Approach | Notes |
|---|---|---|---|
| **1** | CSV, TSV | dialect sniffing + streaming profiler (DuckDB fast path) | the bulk of both portals |
| **1** | RDF (ttl, nt, nq, rdf/xml, jsonld) | stream into a `void:` partition summary | reuses parsing we already have |
| **2** | JSON, JSON Lines | pointer-path frequency tree | keys as fields, depth-capped |
| **2** | XML | streaming `encoding/xml`, element/attribute path tree | XSD ignored; structure from instances |
| **2** | Excel (xlsx) | per-sheet tabular profiling | header detection is the hard part |
| **3** | Parquet | native schema + column stats | schema is declared, so nearly free |
| **3** | ZIP/GZ/TAR containers | expand, recurse as child distributions | bounded (R-SNF-3) |
| **4** | GeoJSON, Shapefile, GeoPackage | geometry type, CRS, bbox, attribute table | needs a geo dependency decision |
| **4** | WFS/WMS/OGC API endpoints | capabilities document → field list | API, not file: different fetch path |
| **5** | PDF, DOCX, HTML tables | table extraction | low precision; value questionable |
| **5** | SQL dumps, SQLite files | table/column introspection | rare but trivially high fidelity |
| **—** | proprietary/unknown | recorded as `viso:Unknown` with detected media type | never an error |

## 10. Delivery phases

**M1 — Skeleton and catalogue (G1). ✅ implemented.** `cmd/visoto-harvest`, SQLite
state with lease-based claims, the `Source` interface with the `dcat-sparql` and
`ckan` adapters, the `Loader` interface with `bulk-file` and `sparql-update`, and
catalogue graphs plus run provenance written into the triplestore. The signature
sketch and its persistence landed here too, ahead of the profiler that will feed
them, because they gate M3 and cannot be retrofitted.
*Remaining to close:* a live harvest against opendata.swiss — the adapter is
covered by fixture tests, but the portal has not been reached from this
environment.

**M2 — Fetch and CSV/RDF structure (G2, G3).** Blob store, sniffing, the CSV and
RDF profilers, minting, `bulk-file` loader. *Done when:* a distribution's columns,
types and cardinalities are queryable and reachable from its `dcat:Dataset`.

**M3 — Signatures (G4).** Sketches in the profiling pass, signature minting, the
similarity batch job. *Done when:* "which other distributions share a column with
this one" returns sensible answers on a 1 000-distribution sample.

**M4 — Incrementality and diffing (G5).** Conditional fetch, content-hash skip,
retention, structural diff between runs. *Done when:* a second run over an
unchanged catalogue writes no triples and completes in minutes.

**M5 — Visoto surfaces.** Templates and icons for `viso:Structure`, `viso:Field`,
`viso:Signature`; the pipeline endpoint in the endpoint switcher.

**M6 — Classification (G6).** Tier 1 validators, the gold set, then tier 2 on the
residue.

Formats beyond phase 1 (§9) slot in after M2 without interface changes.

## 11. Open decisions

| # | Decision | Options | Leaning |
|---|---|---|---|
| D1 | Blob retention after profiling | keep all / keep hash only / LRU cache with a size cap | LRU cache: re-profiling without re-downloading is worth real disk, unbounded storage is not — and it sidesteps the redistribution question |
| D2 | Base IRI | `visoto.hutzli.org/id/` / a purl / a dedicated domain | **decide before M2.** Configurable as `base_iri`, defaulting to `https://visoto.hutzli.org/id/`; changing it after the first load re-mints every IRI in the store |
| D3 | Sample values in the graph | top-k with values / frequencies only / values only for low-cardinality fields | third option: it keeps code lists legible while limiting how much content is republished |
| D4 | Similarity job placement | in-pipeline batch / separate binary / SPARQL-side | in-pipeline batch for M3 |
| D5 | DuckDB dependency | required / optional fast path / not used | optional fast path (R-PRF-7) |
| D6 | Own QLever instance vs. an existing endpoint | dedicated / shared with LINDAS mirror | dedicated: re-index cadence differs sharply |
| D7 | Whether the web UI ever triggers a harvest | never / admin-only / open | out of scope for v1, but affects whether state needs multi-writer safety |
| D8 | When to move the job queue off SQLite | never / when one machine is too slow / now | when needed, not now: the seam is in place (R-SCALE-2), and a shared queue without a shared rate limiter (R-SCALE-4) would be a regression |

**Resolved**

| # | Decision | Outcome |
|---|---|---|
| — | Catalogue source | **Both, behind a `Source` interface.** Adapters normalize to DCAT-AP; `dcat-sparql` and `ckan` ship in M1 |
| — | Field signatures in v1 | **Yes.** Persisted from the first run (§6.4); they cannot be retrofitted without re-downloading the corpus |
| — | Ingest DCAT into the triplestore | **Yes.** Catalogue graphs per source and run, with structure hanging off the same `dcat:Distribution` IRIs (§5.3) |
| — | Format scope | **Roadmapped** (§9). Phase 1 is CSV/TSV and RDF |

## 12. Risks

- **Interface churn from late formats.** Mitigated by writing the locator
  abstraction (R-MOD-1) against XML and JSON on paper before M2 ships.
- **Politeness failures.** A naive worker pool will get the harvester blocked by
  opendata.swiss. R-FET-5 is not optional.
- **Signature quality.** Thresholds that are too loose produce a hairball of false
  join candidates and destroy trust in the feature. R-SIG-4 keeps them tunable and
  recorded; M3's done-criterion is a manual review of a sample.
- **Classification without evaluation.** Shipping tier 2 before the gold set
  (R-CLS-6) would put unmeasurable guesses into the graph next to measured facts.
- **Graph bloat from projections and history.** R-MOD-4 and R-VER-3 both exist to
  be turned down.

## Appendix — queries the model has to answer

```sparql
# Every column of every CSV distribution in a dataset
SELECT ?dist ?name ?dt ?nulls WHERE {
  <…/dataset/bevoelkerung> dcat:distribution ?dist .
  ?dist viso:hasStructure/viso:field ?f .
  ?f viso:name ?name ; viso:nullCount ?nulls ;
     viso:datatypeHit [ viso:datatype ?dt ; viso:ratio ?r ] .
  FILTER(?r > 0.95)
}

# Datasets sharing a column with this one (join candidates)
SELECT DISTINCT ?otherDataset ?name WHERE {
  <…/resource/a1b2> viso:hasStructure/viso:field/viso:signature ?sig .
  ?other viso:signature ?sig ; viso:name ?name .
  ?otherStruct viso:field ?other .
  ?otherDist viso:hasStructure ?otherStruct .
  ?otherDataset dcat:distribution ?otherDist .
  FILTER(?otherDataset != <…/dataset/bevoelkerung>)
}

# Which publishers expose municipality identifiers, and in how many datasets
SELECT ?publisher (COUNT(DISTINCT ?dataset) AS ?n) WHERE {
  ?sig viso:annotation [ viso:semanticType <https://ld.admin.ch/municipality> ;
                         viso:confidence ?c ] .
  FILTER(?c > 0.8)
  ?field viso:signature ?sig .
  ?struct viso:field ?field .
  ?dist viso:hasStructure ?struct .
  ?dataset dcat:distribution ?dist ; dct:publisher ?publisher .
} GROUP BY ?publisher ORDER BY DESC(?n)

# Distributions whose declared media type is a lie
SELECT ?dist ?declared ?detected WHERE {
  ?dist viso:declaredMediaType ?declared ; viso:detectedMediaType ?detected .
  FILTER(?declared != ?detected)
}
```
