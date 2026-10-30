import { describe, expect, it } from 'vitest';
import { DeadLetterReplayStatus, DryRunOutcome, EditBlock, EditField } from '../../../domain';
import { ReplayDraft, ReplayPhase } from './ReplayDraft';
import { ReplayTargets } from './ReplayTargets';

const original = { key: 'k1', value: { items: null }, headers: [], rawBytes: false, masked: true };

describe('ReplayDraft', () => {
  it('edits, validates and builds the edit and local diff', () => {
    let d = ReplayDraft.of(original);
    expect(d.canSubmit).toBe(true);
    expect(d.edit).toBeNull();
    d = d.type(EditField.Value, '{"items": [');
    expect(d.valid).toBe(false);
    expect(d.canSubmit).toBe(false);
    d = d.type(EditField.Value, '{"items": []}');
    expect(d.edit).toEqual({ value: { items: [] } });
    expect(d.diff).toEqual([{ path: '/value/items', op: 'replace' }]);
  });

  it('moves through dry run, replay and the approval wait', () => {
    const d = ReplayDraft.of(original).withNote('fix items');
    const busy = d.busy();
    expect(busy.canSubmit).toBe(false);
    const preview = { status: DeadLetterReplayStatus.Preview, diff: [], masked: true, preview: { id: 'x', outcome: DryRunOutcome.WouldSend, outputs: [] } };
    const previewed = busy.answered(preview, d.phase);
    expect(previewed.phase).toBe(ReplayPhase.Previewed);
    expect(previewed.canSubmit).toBe(true);
    expect(previewed.note).toBe('fix items');
    expect(previewed.type(EditField.Key, '"k2"').phase).toBe(ReplayPhase.Editing);
    expect(busy.answered(null, ReplayPhase.Previewed).phase).toBe(ReplayPhase.Previewed);
    expect(busy.answered({ status: DeadLetterReplayStatus.PendingApproval, diff: [], masked: false }, d.phase).phase).toBe(ReplayPhase.Pending);
    expect(busy.answered({ status: DeadLetterReplayStatus.Done, diff: [], masked: false }, d.phase).canSubmit).toBe(false);
  });

  it('replays unchanged when the payload cannot be edited', () => {
    const d = ReplayDraft.of({ ...original, rawBytes: true });
    expect(d.texts).toBeNull();
    expect(d.blockedReason).toBe(EditBlock.RawBytes);
    expect(d.type(EditField.Value, 'x')).toBe(d);
    expect(d.canSubmit).toBe(true);
  });

  it('targets a cause by fingerprint and a selection by ids', () => {
    const group = { routeId: 'r1', routeName: 'Orders', step: 'Step 1', pattern: 'p', count: 4, fingerprint: 'fp' } as Parameters<typeof ReplayTargets.group>[0];
    expect(ReplayTargets.group(group)).toMatchObject({ routeId: 'r1', count: 4, filter: { fingerprint: 'fp' } });
    expect(ReplayTargets.selection('r1', 'Orders', ['a', 'b'], { kinds: [] })).toMatchObject({ ids: ['a', 'b'], count: 2 });
  });
});
