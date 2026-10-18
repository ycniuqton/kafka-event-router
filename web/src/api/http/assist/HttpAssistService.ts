import type { ValidationIssue } from '../../generated';
import { StepLine, StepTarget, type InlineEvent, type JsonValue, type TransformStep } from '../../../domain';
import { EventPath, StepMapper } from '../../../domain/routeConfig/StepMapper';
import {
  AssistService,
  type AssistContext,
  type InferredMapping,
  type MappingProposal,
  type MappingReason,
  type StepExplanation,
  type StepProposal,
} from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import {
  AssistEndpoint,
  BlockingSeverity,
  MaxAssistSamples,
  type ExplanationDto,
  type ExportJsDto,
  type InferMappingDto,
  type InferRequestDto,
  type ProposalDto,
  type ProposeRequestDto,
} from './AssistDtos';

/** Pointer prefix of proposal issues (`/steps/{i}/…`). */
const StepIssuePath = { prefix: '/steps/', separator: '/' } as const;

/**
 * `services.assist` over `/assist/*`. Proposals are mapped to studio steps; steps with blocking
 * issues, or kinds the studio cannot show yet, are left out (nothing is applied without Accept).
 */
export class HttpAssistService extends AssistService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async propose(instruction: string, context: AssistContext): Promise<StepProposal[]> {
    const body: ProposeRequestDto = {
      instruction,
      currentSteps: context.steps.map((s) => StepMapper.toStep(s)),
      fields: context.fields.map((f) => EventPath.toDocument(f)),
      samples: context.samples?.slice(0, MaxAssistSamples),
      connectionId: context.connectionId,
    };
    const res = await this.context.api.post<ProposalDto>(AssistEndpoint.steps, body);
    return ProposalMapper.proposals(res);
  }

  async explain(steps: TransformStep[]): Promise<StepExplanation> {
    const res = await this.context.api.post<ExplanationDto>(AssistEndpoint.explain, { steps: steps.map((s) => StepMapper.toStep(s)) });
    return { summary: res.summary, bullets: res.bullets ?? [] };
  }

  async inferMapping(input: InlineEvent, desired: JsonValue): Promise<InferredMapping> {
    const body: InferRequestDto = { input, desiredOutput: desired };
    const res = await this.context.api.post<InferMappingDto>(AssistEndpoint.inferMapping, body);
    return InferredMappingMapper.of(res);
  }

  async exportJs(steps: TransformStep[]): Promise<TransformStep> {
    const res = await this.context.api.post<ExportJsDto>(AssistEndpoint.exportJs, { steps: steps.map((s) => StepMapper.toStep(s)) });
    const step = StepMapper.fromStep(res.step);
    if (!step) throw new Error(ExportJsText.unreadable);
    return step;
  }
}

/** Texts of export-to-JS failures the server does not phrase. */
export const ExportJsText = { unreadable: 'The exported step could not be read.' } as const;

/** Converts a proposal response into studio proposals; steps with blocking issues keep the issue (Accept disabled). */
export class ProposalMapper {
  static proposals(res: Pick<ProposalDto, 'steps' | 'issues'> & { diff?: ProposalDto['diff'] }): StepProposal[] {
    const blocked = ProposalMapper.blockedIssues(res.issues ?? []);
    return res.steps.flatMap((step, i) => {
      const mapped = StepMapper.fromStep(step);
      if (!mapped) return [];
      const proposal: StepProposal = { text: res.diff?.added[i] || StepLine.of(mapped), step: mapped };
      const issue = blocked.get(i);
      return [issue ? { ...proposal, issue } : proposal];
    });
  }

  private static blockedIssues(issues: ValidationIssue[]): Map<number, string> {
    const out = new Map<number, string>();
    for (const issue of issues) {
      if (issue.severity !== BlockingSeverity || !issue.path.startsWith(StepIssuePath.prefix)) continue;
      const index = Number(issue.path.slice(StepIssuePath.prefix.length).split(StepIssuePath.separator)[0]);
      if (!out.has(index)) out.set(index, issue.message);
    }
    return out;
  }
}

/** Maps an infer-mapping response: each step gets the confidence of the target it writes (removals are certain). */
export class InferredMappingMapper {
  static readonly certain = 1;

  static of(res: InferMappingDto): InferredMapping {
    const mappings: MappingProposal[] = res.mappings.map((m) => ({ ...m, target: EventPath.fromDocument(m.target), source: m.source && EventPath.fromDocument(m.source), reason: m.reason as MappingReason }));
    const proposals = ProposalMapper.proposals({ steps: res.steps, issues: res.issues }).map((p) => ({ ...p, confidence: InferredMappingMapper.confidence(p.step, mappings) }));
    return { proposals, mappings, unmapped: res.unmapped.map((u) => EventPath.fromDocument(u)) };
  }

  static confidence(step: TransformStep, mappings: MappingProposal[]): number {
    const target = StepTarget.of(step);
    return mappings.find((m) => m.target === target)?.confidence ?? InferredMappingMapper.certain;
  }
}
