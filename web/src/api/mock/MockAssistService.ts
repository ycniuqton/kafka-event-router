import { StepKindLabel, TransformStepKind, type InlineEvent, type JsonValue, type TransformStep } from '../../domain';
import { AssistService, type AssistContext, type InferredMapping, type StepExplanation, type StepProposal } from '../contracts';
import { MockMappingInferrer } from './MockMappingInferrer';
import type { MockBackend } from './MockBackend';
import { MockJsExporter } from './MockJsExporter';
import { ProposalRuleSet } from './MockProposalRules';

/** Texts of the mock explanation (the design's canned JS summary). */
export const MockExplainText = {
  js: 'Adds a total field (sum of price × qty over items) and a processedAt timestamp, keeping every other field. Fails when an event has no items array.',
  chain: (count: number) => `Runs ${count} steps in order.`,
  notObject: 'Paste the desired output as a JSON object.',
} as const;

/** Mock assistant: the design's keyword matcher and a canned explanation (no model call). */
export class MockAssistService extends AssistService {
  private readonly backend: MockBackend;
  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  propose(instruction: string, _context: AssistContext): Promise<StepProposal[]> {
    return this.backend.reply(ProposalRuleSet.rules.flatMap((rule) => rule.match(instruction)));
  }

  explain(steps: TransformStep[]): Promise<StepExplanation> {
    if (steps.length === 1 && steps[0].kind === TransformStepKind.Js) return this.backend.reply({ summary: MockExplainText.js, bullets: [] });
    const bullets = steps.map((s) => [StepKindLabel[s.kind], s.field].filter(Boolean).join(' '));
    return this.backend.reply({ summary: MockExplainText.chain(steps.length), bullets });
  }

  inferMapping(input: InlineEvent, desired: JsonValue): Promise<InferredMapping> {
    if (typeof desired !== 'object' || desired === null || Array.isArray(desired)) return this.backend.fail(MockExplainText.notObject);
    return this.backend.reply(MockMappingInferrer.infer(input.value, desired));
  }

  exportJs(steps: TransformStep[]): Promise<TransformStep> {
    try {
      return this.backend.reply(MockJsExporter.export(steps));
    } catch (e) {
      return this.backend.fail((e as Error).message);
    }
  }
}
