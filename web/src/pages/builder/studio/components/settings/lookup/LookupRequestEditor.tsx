import type { ComponentType } from 'react';
import { Button, ButtonSize, ButtonVariant, ControlSize, SegmentedControl, SegmentSize, TextArea, TextInput } from '../../../../../../components/ui';
import { LookupHttpMethod, LookupRedisOp, type LookupKeyValue, type LookupRequestDraft } from '../../../../../../domain';
import { LookupInputControl, LookupRequestFormSpec, type LookupRequestInput } from '../../../model/LookupRequestFormSpec';
import styles from '../../../library/Library.module.css';

type ControlProps = { request: LookupRequestDraft; input: LookupRequestInput; onChange(next: LookupRequestDraft): void };

/** Immutable edits of a request member. */
class RequestPatch {
  static readonly lineBreak = '\n';

  static set(r: LookupRequestDraft, member: LookupRequestInput['member'], value: unknown): LookupRequestDraft {
    return { ...r, [member]: value };
  }

  static rows(r: LookupRequestDraft, member: LookupRequestInput['member']): LookupKeyValue[] {
    return (r[member] as LookupKeyValue[] | undefined) ?? [];
  }
}

function TextControl({ request, input, onChange }: ControlProps) {
  return (
    <TextInput controlSize={ControlSize.Sm} mono sunken placeholder={input.placeholder} aria-label={input.label} title={input.label}
      value={String(request[input.member] ?? '')} onChange={(e) => onChange(RequestPatch.set(request, input.member, e.target.value))} />
  );
}

function CodeControl({ request, input, onChange }: ControlProps) {
  return (
    <TextArea controlSize={ControlSize.Sm} mono rows={3} placeholder={input.placeholder} aria-label={input.label}
      value={String(request[input.member] ?? '')} onChange={(e) => onChange(RequestPatch.set(request, input.member, e.target.value))} />
  );
}

function MethodControl({ request, input, onChange }: ControlProps) {
  return <SegmentedControl size={SegmentSize.Xs} options={Object.values(LookupHttpMethod)} value={request.method} ariaLabel={input.label} onChange={(m) => onChange(RequestPatch.set(request, input.member, m))} />;
}

function RedisOpControl({ request, input, onChange }: ControlProps) {
  return <SegmentedControl size={SegmentSize.Xs} mono options={Object.values(LookupRedisOp)} value={request.op} ariaLabel={input.label} onChange={(op) => onChange(RequestPatch.set(request, input.member, op))} />;
}

/** Keys as one template per line. */
function LinesControl({ request, input, onChange }: ControlProps) {
  return (
    <TextArea controlSize={ControlSize.Sm} mono rows={3} placeholder={input.placeholder} aria-label={input.label}
      value={request.keys.join(RequestPatch.lineBreak)} onChange={(e) => onChange(RequestPatch.set(request, input.member, e.target.value.split(RequestPatch.lineBreak)))} />
  );
}

/** Name → template rows (query, headers, SQL params) with add / remove. */
function RowsControl({ request, input, onChange }: ControlProps) {
  const rows = RequestPatch.rows(request, input.member);
  const write = (next: LookupKeyValue[]) => onChange(RequestPatch.set(request, input.member, next));
  return (
    <div className={styles.settings}>
      {rows.map((row, i) => (
        <div key={i} className={styles.row}>
          <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder={input.placeholder} aria-label={`${input.label} name`} value={row.key}
            onChange={(e) => write(rows.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))} />
          <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder="${value.id}" aria-label={`${input.label} value`} value={row.value}
            onChange={(e) => write(rows.map((r, j) => (j === i ? { ...r, value: e.target.value } : r)))} />
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title="Remove" onClick={() => write(rows.filter((_, j) => j !== i))}>×</Button>
        </div>
      ))}
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => write([...rows, { key: '', value: '' }])}>+ {input.label}</Button>
    </div>
  );
}

/** Control of each input kind. */
const LookupInputControls: Record<LookupInputControl, ComponentType<ControlProps>> = {
  [LookupInputControl.Text]: TextControl,
  [LookupInputControl.Code]: CodeControl,
  [LookupInputControl.Method]: MethodControl,
  [LookupInputControl.RedisOp]: RedisOpControl,
  [LookupInputControl.Rows]: RowsControl,
  [LookupInputControl.Lines]: LinesControl,
};

/** The request template of a lookup source step, rendered from `LookupRequestFormSpec` for its request kind. */
export function LookupRequestEditor({ request, onChange }: { request: LookupRequestDraft; onChange(next: LookupRequestDraft): void }) {
  return (
    <div className={styles.settings}>
      {LookupRequestFormSpec.inputs(request).map((input) => {
        const Control = LookupInputControls[input.control];
        return (
          <label key={input.member} className={styles.settings}>
            <span className={styles.muted}>{input.label}</span>
            <Control request={request} input={input} onChange={onChange} />
          </label>
        );
      })}
      <span className={styles.muted}>{LookupRequestFormSpec.hint}</span>
    </div>
  );
}
