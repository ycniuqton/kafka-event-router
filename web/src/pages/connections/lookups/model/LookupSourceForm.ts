import type {
  LookupHttpSettings,
  LookupLimits,
  LookupPostgresSettings,
  LookupRedisSettings,
  LookupSecretField,
  LookupSourceDraft,
  LookupStaticSettings,
  LookupFieldErrors,
} from '../../../../domain';
import type { SecretValue } from '../../model/ConnectionForm';

/** Steps of the lookup source panel (design §2 LS-02). */
export const LookupFormStep = { Basics: 0, Kind: 1, Limits: 2 } as const;
export type LookupFormStep = (typeof LookupFormStep)[keyof typeof LookupFormStep];

export const LookupFormStepLabels = ['Basics', 'Kind settings', 'Limits'] as const;

/** Immutable updates of the draft (each returns a new draft). */
export class LookupDraftPatch {
  static set<K extends keyof LookupSourceDraft>(d: LookupSourceDraft, key: K, value: LookupSourceDraft[K]): LookupSourceDraft {
    return { ...d, [key]: value };
  }

  static http(d: LookupSourceDraft, patch: Partial<LookupHttpSettings>): LookupSourceDraft {
    return { ...d, http: { ...d.http, ...patch } };
  }

  static httpAuth(d: LookupSourceDraft, patch: Partial<LookupHttpSettings['auth']>): LookupSourceDraft {
    return LookupDraftPatch.http(d, { auth: { ...d.http.auth, ...patch } });
  }

  static httpTls(d: LookupSourceDraft, patch: Partial<LookupHttpSettings['tls']>): LookupSourceDraft {
    return LookupDraftPatch.http(d, { tls: { ...d.http.tls, ...patch } });
  }

  static redis(d: LookupSourceDraft, patch: Partial<LookupRedisSettings>): LookupSourceDraft {
    return { ...d, redis: { ...d.redis, ...patch } };
  }

  static postgres(d: LookupSourceDraft, patch: Partial<LookupPostgresSettings>): LookupSourceDraft {
    return { ...d, postgres: { ...d.postgres, ...patch } };
  }

  static staticTable(d: LookupSourceDraft, patch: Partial<LookupStaticSettings>): LookupSourceDraft {
    return { ...d, static: { ...d.static, ...patch } };
  }

  static limits(d: LookupSourceDraft, patch: Partial<LookupLimits>): LookupSourceDraft {
    return { ...d, limits: { ...d.limits, ...patch } };
  }

  static secret(d: LookupSourceDraft, field: LookupSecretField, value: string): LookupSourceDraft {
    return { ...d, secrets: { ...d.secrets, [field]: value } };
  }
}

/** Secrets as the write-only input shows them: stored (•••••• + Replace) until the user types a replacement. */
export class LookupSecretView {
  static of(d: LookupSourceDraft, stored: readonly LookupSecretField[], field: LookupSecretField): SecretValue {
    const typed = d.secrets[field];
    return { value: typed ?? '', stored: typed === undefined && stored.includes(field) };
  }
}

/** Which panel step shows the field behind an error pointer. */
export class LookupErrorSteps {
  private static readonly prefixes: [string, LookupFormStep][] = [
    ['/limits', LookupFormStep.Limits],
    ['/http', LookupFormStep.Kind],
    ['/redis', LookupFormStep.Kind],
    ['/postgres', LookupFormStep.Kind],
    ['/static', LookupFormStep.Kind],
    ['/secrets', LookupFormStep.Kind],
  ];

  static stepOf(pointer: string): LookupFormStep {
    return LookupErrorSteps.prefixes.find(([p]) => pointer.startsWith(p))?.[1] ?? LookupFormStep.Basics;
  }

  static firstStep(errors: LookupFieldErrors): LookupFormStep | null {
    const steps = Object.keys(errors).map((p) => LookupErrorSteps.stepOf(p));
    return steps.length ? (Math.min(...steps) as LookupFormStep) : null;
  }

  static onStep(errors: LookupFieldErrors, step: LookupFormStep): LookupFieldErrors {
    return Object.fromEntries(Object.entries(errors).filter(([p]) => LookupErrorSteps.stepOf(p) === step));
  }
}

/** Props of every step view of the panel. */
export type LookupFormStepProps = {
  draft: LookupSourceDraft;
  stored: readonly LookupSecretField[];
  editing: boolean;
  teamName: string;
  errors: LookupFieldErrors;
  update: (next: LookupSourceDraft) => void;
};
