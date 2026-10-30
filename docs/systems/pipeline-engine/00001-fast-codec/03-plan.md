# Pipeline Engine — Plan (00001-fast-codec)

Incremental version on top of `00000-init`. **Unchanged:** every item PE-01..PE-112 in `00000-init/03-plan.md`
keeps its state there. This plan adds only the fast JSON codec items (`PE-FC-xx`). Mark the step you start `[~]`,
finished `[x]`; never delete items.

## Analysis (compile time)
- [x] PE-FC-01 `projection.Recorder` on `compilectx.Scope`: records every parsed path, step kind, filter node type, value source kind, formula field set and script use
- [x] PE-FC-02 `formula.Program.Fields()` (AST walk) so formula field references join the recorded set
- [x] PE-FC-03 `projection.Analyzer` → `Plan` (field tree + per-route fallback reason); allow-lists of path-bounded step kinds, node types, value sources; source + destination formats must be JSON

## Byte-level codec (`pkg/engine/codec/fastjson`)
- [x] PE-FC-10 Scanner → tape (one pass: validate, spans, canonical flags); rejects anything unusual so the event takes the tree path
- [x] PE-FC-11 Materializer: tape → Go values identical to `codec.TreeReader`
- [x] PE-FC-12 CanonicalWriter: tape → bytes identical to `TreeWriter(TreeReader(raw))` (sorted keys, last-wins duplicates, canonical strings/numbers, raw copy of canonical subtrees)
- [x] PE-FC-13 ValueWriter: Go tree → bytes identical to `codec.TreeWriter` (append-based; unknown types delegate)

## Projection + patch encode (`pkg/engine/projection`)
- [x] PE-FC-20 Projector: decode only the planned fields into a partial map tree
- [x] PE-FC-21 Patcher: splice changed / added / removed members into the canonical original; passthrough returns the input bytes when unchanged and canonical

## Pipeline integration
- [x] PE-FC-30 `engine.CodecMode` (fast | tree), `engine.RecordRunner` (fused decode → process → encode), `engine.CodecPlan` (mode + fallback reason)
- [x] PE-FC-31 `compile`: plan at compile, record strategies (tree / projected), any fast-path error re-runs the record on the tree path
- [x] PE-FC-32 `OutputShaper.EncodeWith` (pluggable value encoder, key/headers/size unchanged)
- [x] PE-FC-33 Runner `RecordProcessor` uses `RecordRunner`; `ROUTER_ENGINE_CODEC` env (default fast); per-route codec gauge + log with fallback reason

## Safety
- [x] PE-FC-40 Differential test: every golden route (engine + preview goldens) × fixed + randomized events through fast and tree, identical outputs and errors
- [x] PE-FC-41 Byte-level property tests: canonical writer vs tree round trip (nested, arrays, unicode/escapes, numbers, duplicates, whitespace, invalid JSON)
- [x] PE-FC-42 Go fuzz targets (`FuzzCanonicalWriter`, `FuzzFastVsTree`)
- [x] PE-FC-43 All existing tests and goldens pass unchanged

## Measure
- [x] PE-FC-50 S1 matrix fast vs tree (`s1-fast-codec.sh` → `s1-bench.sh` with `ENGINE_CODEC`), `test/stress/results/s1-fast-codec.json` — filter-only 1 KB 16.0k → 111.7k, recipe-5 1 KB 14.4k → 44.1k, 10 KB 5–10×, 100 KB 5–10×; mapper / JS unchanged (fallback); see 02-design §7
- [x] PE-FC-51 Docs: 01-requirements, 02-design (flows), results table

## Follow-ups (integration / later)
- [x] PE-FC-60 Integration: rerun S2 (p1, p12 mif 5000) and S5 (baseline, steps 5/20) end to end with `ROUTER_ENGINE_CODEC=fast` vs `tree`; check zero loss / duplicates and that `router_route_codec_total{fallback}` matches the routes — done (wave 2, clean-host window, 1-min explore steps): S2 p12 auto fast 140k vs tree 80k; p1 (mif 5000) 80k vs 30k; S5 baseline 90–160k vs 72.5k, steps 5 90–110k vs 57.5k, steps 20 57.5–72.5k vs 40k, JS 6.5k both (fallback `script`); 0 lost / 0 dup in every run; fast vs tree output bytes identical (`results/wave2-bytes/compare.txt`, 1882 records); per-route codec log in `results/wave2-codec-log.txt`. Results `test/stress/results/wave2-s2-*.json`, `wave2-s5-*.json`
- [ ] PE-FC-61 Export `CodecStats.Fallbacks` (records re-run on the tree codec) as a per-route counter
- [ ] PE-FC-62 Reduce per-event allocations of `Process` / `branch.run` (shared by both codecs) and recipe step cost (now the recipe-chain bottleneck)
- [ ] PE-FC-63 Fast path for the visual mapper (`start: empty`: partial decode of the read paths, tree encode of the new value)
- [x] PE-FC-64 Integration with 00002-lookups: the `lookup` step kind gained a source mode whose request placeholders were parsed outside `Scope.Path`, so the fast path missed them (wrong lookups on projected events). `enrich.RequestAccess` now records the step's declared reads (`step.FieldAccessor`) into the projection recorder; `map[].to` writes already go through `Scope.Path`. Lookup routes run fast; differential test `codec_lookup_test.go` (http/redis/sql/static, all template positions, miss/error policies, cache) + golden `lookup_source` in `TestFastCodecGoldenParity`
