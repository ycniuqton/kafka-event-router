import { MaskMode, StepFactory, TransformStepKind } from '../../domain';
import type { StepProposal } from '../contracts';

/** One keyword pattern of the mock assistant; returns every proposal it finds in the prompt. */
export abstract class ProposalRule {
  abstract match(prompt: string): StepProposal[];
}

/** Rule driven by a global regex; each match becomes one proposal. */
abstract class RegexProposalRule extends ProposalRule {
  protected abstract readonly pattern: RegExp;
  protected abstract build(groups: string[]): StepProposal;

  match(prompt: string): StepProposal[] {
    return [...prompt.matchAll(this.pattern)].map((m) => this.build(m.slice(1)));
  }
}

class RenameRule extends RegexProposalRule {
  protected readonly pattern = /rename\s+([\w.]+)\s+(?:to|→|->)\s+([\w.]+)/gi;
  protected build([from, to]: string[]): StepProposal {
    return { text: `Rename ${from} → ${to}`, step: StepFactory.create(TransformStepKind.Rename, { field: from, argument: to }) };
  }
}

class MaskRule extends RegexProposalRule {
  protected readonly pattern = /(?:hide|mask)\s+(?:the\s+)?([\w.]+)/gi;
  static readonly emailHint = /mail/;
  protected build([field]: string[]): StepProposal {
    const isEmail = MaskRule.emailHint.test(field);
    const style = isEmail ? 'email style' : 'keep last 4';
    const step = StepFactory.create(TransformStepKind.Mask, { field, maskMode: isEmail ? MaskMode.Email : MaskMode.Last4 });
    return { text: `Mask ${field} (${style})`, step };
  }
}

class RemoveRule extends RegexProposalRule {
  protected readonly pattern = /(?:remove|drop)\s+(?:the\s+)?([\w.]+)/gi;
  protected build([field]: string[]): StepProposal {
    return { text: `Remove ${field}`, step: StepFactory.create(TransformStepKind.Remove, { field }) };
  }
}

class HashRule extends RegexProposalRule {
  protected readonly pattern = /hash\s+(?:the\s+)?([\w.]+)/gi;
  protected build([field]: string[]): StepProposal {
    return { text: `Hash ${field}`, step: StepFactory.create(TransformStepKind.Hash, { field }) };
  }
}

class KeyRule extends RegexProposalRule {
  protected readonly pattern = /key\s+(?:from|by|to)\s+([\w.]+)/gi;
  protected build([field]: string[]): StepProposal {
    return { text: `Set key from ${field}`, step: StepFactory.create(TransformStepKind.SetKey, { field }) };
  }
}

class OnlyOrderFieldsRule extends ProposalRule {
  static readonly trigger = /only keep order/i;
  static readonly nonOrderFields = ['country', 'currency'];
  match(prompt: string): StepProposal[] {
    if (!OnlyOrderFieldsRule.trigger.test(prompt)) return [];
    return OnlyOrderFieldsRule.nonOrderFields.map((field) => ({
      text: `Remove ${field} (not an order field)`,
      step: StepFactory.create(TransformStepKind.Remove, { field }),
    }));
  }
}

/** The design's keyword matcher, in its evaluation order. */
export class ProposalRuleSet {
  static readonly rules: ProposalRule[] = [
    new RenameRule(),
    new MaskRule(),
    new RemoveRule(),
    new HashRule(),
    new KeyRule(),
    new OnlyOrderFieldsRule(),
  ];
}
