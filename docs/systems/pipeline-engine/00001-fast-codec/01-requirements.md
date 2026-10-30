# Pipeline Engine — Requirements (00001-fast-codec)

Incremental version on top of `00000-init`. **Unchanged:** every requirement of `00000-init/01-requirements.md`
(FR-01..FR-xx, NFRs) still applies. This version adds the fast JSON record path.

## Business intent
The S1/S2 stress tests (`route-runtime/00001-stress-test`) showed ~87 % of the CPU of a 1 KB JSON event goes to
decoding the whole value into a tree and re-encoding it, capping a core at ~15–18k events/s against an NFR of
100k. Most routes read or write a handful of fields, so the worker should only decode those fields and splice
the changes back into the original bytes — without any visible change in output.

## Functional requirements
- FC-01 At compile time the engine determines, per route version, the set of value field paths it can read or
  write: filter conditions, recipe steps (nested `if_else` / library steps included), value sources, formula
  field references, destination key-from-field.
- FC-02 For JSON source values the worker decodes only those paths (partial decode); the rest of the record is
  scanned and validated but not turned into Go values.
- FC-03 Outputs are produced by patch-encoding: changed, added and removed members are spliced into the original
  value; unchanged members are copied.
- FC-04 Passthrough: when a value is unchanged and the input is already in output form, the output bytes are the
  input bytes (no encode).
- FC-05 Automatic fallback to the tree codec (today's path) for: JS steps and `expr` filter nodes, whole-value
  steps (`keep_only`, `wrap`, `unwrap`, `map`), any step kind / filter node / value source not known to be
  path-bounded (new kinds fall back until allow-listed), formulas whose fields cannot be listed, paths that
  address the whole value, non-JSON source or destination formats.
- FC-06 Per record fallback: values that are not a JSON object, invalid JSON, inputs over the scanner limits, and
  any record whose fast run fails (decode, process or encode error) re-run on the tree codec.
- FC-07 `ROUTER_ENGINE_CODEC=fast|tree` on the worker (default `fast`); `tree` restores today's behaviour.
- FC-08 The per-route codec and fallback reason are logged at compile (`runtime: route codec`) and counted in
  `router_route_codec_total{codec,fallback}`.
- FC-09 Preview, batch (`routerctl`), test cases and DLQ replay keep the tree codec (they trace or inspect values).

## Safety requirements
- SF-01 For every record the fast path handles, output records (key, value bytes, headers, topic, partition,
  timestamp, branch) and errors are byte-identical to the tree codec. The tree codec sorts object keys, so
  "identical" wins over "keep the original field order": the fast path also emits sorted keys.
- SF-02 A differential test runs every golden route (engine and preview goldens) and synthetic routes covering
  every allow-listed step kind through both codecs with fixed and randomized events (nested objects, arrays,
  unicode and escapes, big ints / floats / exponents, duplicate keys, whitespace, invalid JSON).
- SF-03 Go fuzz targets compare the byte-level codec with the tree reader/writer and the fast pipeline with the
  tree pipeline.
- SF-04 All existing tests and goldens pass unchanged.

## Non-functional requirements
- NF-01 Measure the S1 matrix (filter-only, recipe chain, mapper, JS × 1/10/100 KB) for both codecs; record in
  `test/stress/results/s1-fast-codec.json`.
- NF-02 Target 100k events/s per core for 1 KB filter / recipe routes.
- NF-03 No new per-record allocations proportional to payload size besides the output buffer.

## Out of scope
- Fast path for JS, the visual mapper and non-JSON formats (Avro / Protobuf keep their codecs).
- Preserving the producer's field order (would break SF-01).
