import { StepConstant, StepFactory, StepFieldList, StepKindLabel, TransformStepKind, ValueSourceKind, type TransformStep } from '../../domain';

/** One recipe kind written as JS statements over `v` (event.value) and the `at` path helpers. */
type JsWriter = (step: TransformStep) => string[];

/** Path helpers the exported code starts with (nested `a.b.c` paths under event.value). */
const JsPathHelpers = [
  '  const keys = (p) => p.split(".");',
  '  const parent = (o, p, make) => keys(p).slice(0, -1).reduce((t, k) => (t == null ? t : (make && (t[k] == null || typeof t[k] !== "object") ? (t[k] = {}) : t[k])), o);',
  '  const last = (p) => keys(p)[keys(p).length - 1];',
  '  const get = (o, p) => { const t = parent(o, p, false); return t == null ? undefined : t[last(p)]; };',
  '  const has = (o, p) => { const t = parent(o, p, false); return t != null && Object.prototype.hasOwnProperty.call(t, last(p)); };',
  '  const put = (o, p, x) => { parent(o, p, true)[last(p)] = x; };',
  '  const del = (o, p) => { const t = parent(o, p, false); if (t != null) delete t[last(p)]; };',
];

/** Mock of `/assist/export-js`: recipe steps → one equivalent JS step (unsupported kinds are rejected). */
export class MockJsExporter {
  static readonly unsupported = (label: string) => `${label} steps cannot be exported to JavaScript yet.`;

  private static readonly q = (text: string) => JSON.stringify(text);

  private static readonly writers: Partial<Record<TransformStepKind, JsWriter>> = {
    [TransformStepKind.Rename]: (s) => [`  if (has(v, ${MockJsExporter.q(s.field)})) { put(v, ${MockJsExporter.q(s.argument)}, get(v, ${MockJsExporter.q(s.field)})); del(v, ${MockJsExporter.q(s.field)}); }`],
    [TransformStepKind.Move]: (s) => [`  if (has(v, ${MockJsExporter.q(s.field)})) { put(v, ${MockJsExporter.q(s.argument)}, get(v, ${MockJsExporter.q(s.field)})); del(v, ${MockJsExporter.q(s.field)}); }`],
    [TransformStepKind.Copy]: (s) => [`  if (has(v, ${MockJsExporter.q(s.field)})) put(v, ${MockJsExporter.q(s.argument)}, structuredClone(get(v, ${MockJsExporter.q(s.field)})));`],
    [TransformStepKind.Remove]: (s) => StepFieldList.of(s).map((f) => `  del(v, ${MockJsExporter.q(f)});`),
    [TransformStepKind.Mask]: (s) => [`  if (has(v, ${MockJsExporter.q(s.field)})) put(v, ${MockJsExporter.q(s.field)}, ctx.mask.${s.maskMode}(get(v, ${MockJsExporter.q(s.field)})));`],
    [TransformStepKind.Hash]: (s) => [`  if (has(v, ${MockJsExporter.q(s.field)})) put(v, ${MockJsExporter.q(s.field)}, ctx.hash(get(v, ${MockJsExporter.q(s.field)})));`],
    [TransformStepKind.Set]: (s) => MockJsExporter.setValue(s),
    [TransformStepKind.DefaultIfMissing]: (s) => [`  if (!has(v, ${MockJsExporter.q(s.field)})) put(v, ${MockJsExporter.q(s.field)}, ${JSON.stringify(StepConstant.parse(s.argument))});`],
  };

  /** Returns the JS step, or throws naming the first step that cannot be exported. */
  static export(steps: TransformStep[]): TransformStep {
    const body: string[] = [];
    for (const step of steps.filter((s) => s.enabled)) {
      const writer = MockJsExporter.writers[step.kind];
      if (!writer || !MockJsExporter.supported(step)) throw new Error(MockJsExporter.unsupported(StepKindLabel[step.kind]));
      body.push(`  // ${StepKindLabel[step.kind]}`, ...writer(step));
    }
    const code = ['function transform(event, ctx) {', '  const v = event.value;', ...JsPathHelpers, ...body, '  return event;', '}'].join('\n');
    return StepFactory.create(TransformStepKind.Js, { code });
  }

  private static supported(step: TransformStep): boolean {
    return step.kind !== TransformStepKind.Set || (step.sourceKind ?? ValueSourceKind.Constant) !== ValueSourceKind.Metadata;
  }

  private static setValue(s: TransformStep): string[] {
    if (s.sourceKind === ValueSourceKind.Field) return [`  put(v, ${MockJsExporter.q(s.field)}, structuredClone(get(v, ${MockJsExporter.q(s.argument)})));`];
    return [`  put(v, ${MockJsExporter.q(s.field)}, ${JSON.stringify(StepConstant.parse(s.argument))});`];
  }
}
