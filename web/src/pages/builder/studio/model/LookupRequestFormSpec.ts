import { LookupRedisOp, LookupRequestKind, type LookupRequestDraft } from '../../../../domain';

/** Controls a lookup request input is rendered with (one generic editor renders a kind's inputs as data). */
export const LookupInputControl = {
  Text: 'text',
  Code: 'code',
  Method: 'method',
  RedisOp: 'redisOp',
  Rows: 'rows',
  Lines: 'lines',
} as const;
export type LookupInputControl = (typeof LookupInputControl)[keyof typeof LookupInputControl];

/** One input of the request form: which request member it edits, how, and when it shows. */
export type LookupRequestInput = {
  member: keyof Omit<LookupRequestDraft, 'kind'>;
  control: LookupInputControl;
  label: string;
  placeholder: string;
  when?: (r: LookupRequestDraft) => boolean;
};

/** Request inputs per request kind (TS-14 pattern: forms as data); templates take `${value.field}` placeholders. */
export class LookupRequestFormSpec {
  static readonly hint = 'Use ${value.customerId} to insert event fields; a lone placeholder keeps its type.';

  static readonly forms: Record<LookupRequestKind, LookupRequestInput[]> = {
    [LookupRequestKind.Http]: [
      { member: 'method', control: LookupInputControl.Method, label: 'Method', placeholder: '' },
      { member: 'path', control: LookupInputControl.Text, label: 'Path', placeholder: '/customers/${value.customerId}' },
      { member: 'query', control: LookupInputControl.Rows, label: 'Query', placeholder: 'fields' },
      { member: 'headers', control: LookupInputControl.Rows, label: 'Headers', placeholder: 'x-tenant' },
      { member: 'body', control: LookupInputControl.Code, label: 'JSON body (POST)', placeholder: '{"id": "${value.customerId}"}' },
    ],
    [LookupRequestKind.Redis]: [
      { member: 'op', control: LookupInputControl.RedisOp, label: 'Command', placeholder: '' },
      { member: 'key', control: LookupInputControl.Text, label: 'Key', placeholder: 'flags:${value.merchantId}', when: (r) => r.op !== LookupRedisOp.MGet },
      { member: 'hashField', control: LookupInputControl.Text, label: 'Hash field', placeholder: 'checkout', when: (r) => r.op === LookupRedisOp.HGet },
      { member: 'keys', control: LookupInputControl.Lines, label: 'Keys (one per line)', placeholder: 'price:${value.sku}', when: (r) => r.op === LookupRedisOp.MGet },
    ],
    [LookupRequestKind.Sql]: [
      { member: 'sql', control: LookupInputControl.Code, label: 'SELECT (read-only, :name parameters)', placeholder: 'SELECT region, tier FROM stores WHERE id = :id' },
      { member: 'params', control: LookupInputControl.Rows, label: 'Parameters', placeholder: 'id' },
    ],
    [LookupRequestKind.Static]: [{ member: 'key', control: LookupInputControl.Text, label: 'Table key', placeholder: '${value.country}' }],
  };

  /** Inputs shown for the request right now. */
  static inputs(r: LookupRequestDraft): LookupRequestInput[] {
    return LookupRequestFormSpec.forms[r.kind].filter((i) => !i.when || i.when(r));
  }
}
