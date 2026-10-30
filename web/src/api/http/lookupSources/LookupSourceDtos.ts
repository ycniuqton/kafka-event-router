import type { LookupKindSettings, LookupLimits, LookupSecrets, LookupSource, LookupSourceKind, LookupTestResult } from '../../../domain';
import type { EnvTag } from '../../../domain';

/** `GET /lookup-sources[/{id}]` body: the domain shape is the wire shape (connections 00001 design §3). */
export type LookupSourceDto = LookupSource;

/** `POST /lookup-sources`, `POST /lookup-sources/test` and the merge-patch base: only the kind's settings, write-only secrets. */
export type LookupSourceInputDto = LookupKindSettings & {
  name: string;
  description: string;
  kind: LookupSourceKind;
  envTag: EnvTag;
  sensitive: boolean;
  owner?: { teamId: string };
  limits: LookupLimits;
  secrets?: LookupSecrets;
};

export type LookupTestResultDto = LookupTestResult;

/** Paths of the lookup sources API (relative to `/api/v1`). */
export class LookupSourceEndpoint {
  static readonly collection = '/lookup-sources';
  static readonly draftTest = '/lookup-sources/test';

  static item(id: string): string {
    return `${LookupSourceEndpoint.collection}/${encodeURIComponent(id)}`;
  }

  static test(id: string): string {
    return `${LookupSourceEndpoint.item(id)}/test`;
  }
}

/** Problem type suffix of a delete blocked by routes (`…/problems/lookup-source-in-use`). */
export const LookupSourceProblem = { InUseSuffix: 'lookup-source-in-use' } as const;

/** Extension members of the in-use problem. */
export type LookupSourceProblemExtras = { routes?: { id: string; name: string }[] };
