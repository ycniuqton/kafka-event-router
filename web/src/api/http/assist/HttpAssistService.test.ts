import { describe, expect, it } from 'vitest';
import { MaskMode, StepFactory, TransformStepKind } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpAssistService } from './HttpAssistService';

function setup() {
  const fetch = new TestFetch();
  return { fetch, service: new HttpAssistService(new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn })) };
}

describe('HttpAssistService', () => {
  it('sends steps and value paths, and maps accept-able proposals', async () => {
    const { fetch, service } = setup();
    fetch.json(200, {
      proposalId: 'p1',
      steps: [
        { kind: 'mask', field: 'value.customer.email', style: 'email' },
        { kind: 'keep_only', fields: ['value.a'] },
        { kind: 'rename', from: 'value.a[', to: 'value.b' },
      ],
      diff: { added: ['Mask customer.email (email style)', 'Keep only a', 'Rename a[ → b'], removed: [], changed: [] },
      issues: [{ code: 'path_invalid', severity: 'error', path: '/steps/2/from', message: 'bad' }],
      provider: 'mock',
    });
    const current = [StepFactory.create(TransformStepKind.Hash, { field: 'cust_id' })];
    const proposals = await service.propose('hide the email', { steps: current, fields: ['customer.email'] });
    expect(fetch.calls[0].url).toBe('/api/v1/assist/steps');
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({
      instruction: 'hide the email',
      currentSteps: [{ kind: 'hash', field: 'value.cust_id' }],
      fields: ['value.customer.email'],
    });
    expect(proposals.map((p) => p.text)).toEqual(['Mask customer.email (email style)', 'Keep only a', 'Rename a[ → b']);
    expect(proposals.map((p) => p.issue)).toEqual([undefined, undefined, 'bad']);
    expect(proposals[0].step).toMatchObject({ kind: TransformStepKind.Mask, field: 'customer.email', maskMode: MaskMode.Email });
    expect(proposals[1].step).toMatchObject({ kind: TransformStepKind.KeepOnly, fields: ['a'] });
  });

  it('infers steps from an example with per-step confidence', async () => {
    const { fetch, service } = setup();
    fetch.json(200, {
      steps: [{ kind: 'rename', from: 'value.cust_id', to: 'value.customerId' }, { kind: 'set', field: 'value.src', source: { from: 'constant', value: 'web' } }],
      mappings: [
        { target: 'value.customerId', source: 'value.cust_id', reason: 'value', confidence: 1 },
        { target: 'value.src', reason: 'constant', confidence: 0.5 },
      ],
      unmapped: ['value.debug'],
      issues: [],
    });
    const res = await service.inferMapping({ key: 'k', value: { cust_id: 'c', debug: true } }, { customerId: 'c', src: 'web' });
    expect(fetch.calls[0].url).toBe('/api/v1/preview/infer-mapping');
    expect(res.proposals.map((p) => p.confidence)).toEqual([1, 0.5]);
    expect(res.proposals[0].step).toMatchObject({ kind: TransformStepKind.Rename, field: 'cust_id', argument: 'customerId' });
    expect(res.mappings[1]).toEqual({ target: 'src', source: undefined, reason: 'constant', confidence: 0.5 });
    expect(res.unmapped).toEqual(['debug']);
  });

  it('explains steps through the API', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { summary: 'Runs code.', bullets: ['Drops some events (returns null).'], provider: 'mock' });
    const js = StepFactory.create(TransformStepKind.Js, { code: 'function transform(e){return null}' });
    expect(await service.explain([js])).toEqual({ summary: 'Runs code.', bullets: ['Drops some events (returns null).'] });
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({ steps: [{ kind: 'js', code: 'function transform(e){return null}' }] });
  });
});
