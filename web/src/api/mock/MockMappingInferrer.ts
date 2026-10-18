import { StepFactory, StepLine, StepTarget, TransformStepKind, ValueSourceKind, type JsonValue, type TransformStep } from '../../domain';
import { MappingReason, type InferredMapping, type MappingProposal, type StepProposal } from '../contracts';

type Leaf = { path: string; name: string; parent: string; value: JsonValue };

/** Confidence levels of the mock matcher chain (mirrors Go `infer`). */
const MockConfidence = { exact: 1, ambiguous: 0.8, name: 0.7, constant: 0.5 } as const;

/** Mock transform by example: equal values first, then normalised names, else constants; unused inputs are removed. */
export class MockMappingInferrer {
  static readonly separator = '.';

  static infer(input: JsonValue, desired: JsonValue): InferredMapping {
    const sources = MockMappingInferrer.leaves(input);
    const targets = MockMappingInferrer.leaves(desired);
    const used = new Map<string, number>();
    const mappings = targets.map((t) => MockMappingInferrer.match(t, sources, used));
    const steps = mappings.flatMap((m) => MockMappingInferrer.step(m, targets, used));
    const removed = MockMappingInferrer.removals(input, targets);
    if (removed.length) steps.push(StepFactory.create(TransformStepKind.Remove, { field: removed[0], fields: removed }));
    const proposals: StepProposal[] = steps.map((step) => ({ text: StepLine.of(step), step, confidence: MockMappingInferrer.confidence(step, mappings) }));
    const unmapped = sources.filter((s) => !used.has(s.path) && !targets.some((t) => t.path === s.path)).map((s) => s.path);
    return { proposals, mappings, unmapped };
  }

  private static match(target: Leaf, sources: Leaf[], used: Map<string, number>): MappingProposal {
    const text = JSON.stringify(target.value);
    const equal = sources.filter((s) => JSON.stringify(s.value) === text);
    const named = sources.filter((s) => MockMappingInferrer.nameKey(s.name) === MockMappingInferrer.nameKey(target.name));
    const pick = equal.find((s) => s.name === target.name) ?? equal[0] ?? named[0];
    if (!pick || (pick.path === target.path && JSON.stringify(pick.value) !== text)) {
      return { target: target.path, reason: MappingReason.Constant, confidence: MockConfidence.constant };
    }
    used.set(pick.path, (used.get(pick.path) ?? 0) + 1);
    if (!equal.includes(pick)) return { target: target.path, source: pick.path, reason: MappingReason.Name, confidence: MockConfidence.name };
    const confidence = equal.length > 1 ? MockConfidence.ambiguous : MockConfidence.exact;
    return { target: target.path, source: pick.path, reason: MappingReason.Value, confidence };
  }

  private static step(m: MappingProposal, targets: Leaf[], used: Map<string, number>): TransformStep[] {
    const target = targets.find((t) => t.path === m.target) as Leaf;
    if (!m.source) return [StepFactory.create(TransformStepKind.Set, { field: m.target, sourceKind: ValueSourceKind.Constant, argument: JSON.stringify(target.value) })];
    if (m.source === m.target) return [];
    const remaining = (used.get(m.source) ?? 1) - 1;
    used.set(m.source, remaining);
    const kept = remaining > 0 || targets.some((t) => t.path === m.source);
    const sameParent = MockMappingInferrer.parent(m.source) === MockMappingInferrer.parent(m.target);
    const kind = kept ? TransformStepKind.Copy : sameParent ? TransformStepKind.Rename : TransformStepKind.Move;
    return [StepFactory.create(kind, { field: m.source, argument: m.target })];
  }

  /** Top-level input nodes the desired output neither keeps nor contains. */
  private static removals(input: JsonValue, targets: Leaf[]): string[] {
    if (!MockMappingInferrer.isObject(input)) return [];
    return Object.keys(input).filter((k) => !targets.some((t) => t.path === k || t.path.startsWith(k + MockMappingInferrer.separator)));
  }

  private static confidence(step: TransformStep, mappings: MappingProposal[]): number {
    const target = StepTarget.of(step);
    return mappings.find((m) => m.target === target)?.confidence ?? MockConfidence.exact;
  }

  private static leaves(value: JsonValue, prefix = ''): Leaf[] {
    if (!MockMappingInferrer.isObject(value) || (prefix && !Object.keys(value).length)) {
      return prefix ? [{ path: prefix, name: prefix.split(MockMappingInferrer.separator).pop() ?? prefix, parent: MockMappingInferrer.parent(prefix), value }] : [];
    }
    return Object.entries(value).flatMap(([k, v]) => MockMappingInferrer.leaves(v, prefix ? prefix + MockMappingInferrer.separator + k : k));
  }

  private static parent(path: string): string {
    return path.split(MockMappingInferrer.separator).slice(0, -1).join(MockMappingInferrer.separator);
  }

  private static nameKey(name: string): string {
    return name.toLowerCase().replace(/[-_\s]/g, '');
  }

  private static isObject(v: JsonValue): v is { [k: string]: JsonValue } {
    return typeof v === 'object' && v !== null && !Array.isArray(v);
  }
}
