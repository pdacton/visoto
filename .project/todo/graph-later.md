# Graph Explorer — deferred ideas

Status: **parked**. Spun out of `graph-layout.md`; IDs are shared with it and stay
stable.

- **GL-27** **Animated layout changes:** node positions move over ~300 ms so the
  user can follow what moved; no animation under `prefers-reduced-motion`. In GE:
  interpolated `setPosition` over frames; Reactodia has `performLayout({ animate })`.
- **GL-28** **Class map** starting view for datasets: classes as nodes with instance
  counts, edges = properties used between their instances, weighted by frequency.
  One aggregate query, async + cached; slow on LINDAS without VoID or statistics.
  Data-side counterpart of the ontology diagram.
- **GL-42** **Share link:** canvas stored server-side in the SQLite file under
  `./data`, opened via `/graph/<id>`. Needs a decision on anonymous writes, plus a
  size cap, TTL and abuse limits; must be a separate route so `/resource` stays a
  pure, cacheable function of the URL. Not encodable in the URL (IRIs blow the
  ~8 KB limit).
