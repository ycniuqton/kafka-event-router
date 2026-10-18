import { createHash } from 'node:crypto';
import { expect, test } from '../../lib/Fixtures';
import { Filter, JsSteps, Steps } from '../../lib/RouteDraft';
import { WorkspaceSettings } from '../../lib/Settings';
import { StudioRunner } from '../../lib/Studio';

/** Transform Studio recipes, JS step, mapper and magic features (UI spec §8, SysDesign §7.4–7.6). */
const Order = { orderId: 'o-1', cust_id: 7, email: 'john@mail.com', card: '4111111111111111', amount: 12.345, country: 'VN', status: 1, customer: { firstName: 'Ada', lastName: 'Lovelace' }, items: [{ sku: 'A' }, { sku: 'B' }] };

test.describe('transform studio · recipes', () => {
  test('TC-TS-01 rename moves the value to the new name', async ({ ws }) => {
    const out = await new StudioRunner(ws).one([Steps.rename('value.cust_id', 'value.customerId')], Order);
    expect(out.customerId).toBe(7);
    expect(out).not.toHaveProperty('cust_id');
  });

  test('TC-TS-02 mask styles: full, keep last 4, email keeps the first letter (j***@mail.com)', async ({ ws }) => {
    const out = await new StudioRunner(ws).one([Steps.mask('value.email', 'email'), Steps.mask('value.card', 'last4'), Steps.mask('value.orderId', 'full')], Order);
    expect(out.email).toBe('j***@mail.com');
    expect(out.card).toBe('************1111');
    expect(out.orderId).toMatch(/^\*+$/);
  });

  test('TC-TS-03 hash is SHA-256 with a secret salt (stable, not the bare digest)', async ({ ws }) => {
    const runner = new StudioRunner(ws);
    const a = await runner.one([{ kind: 'hash', field: 'value.email' }], Order);
    const b = await runner.one([{ kind: 'hash', field: 'value.email' }], Order);
    expect(a.email).toMatch(/^[0-9a-f]{64}$/);
    expect(a.email).toBe(b.email);
    expect(a.email).not.toBe(createHash('sha256').update(Order.email).digest('hex'));
  });

  test('TC-TS-04 remove and keep-only', async ({ ws }) => {
    const runner = new StudioRunner(ws);
    expect(Object.keys(await runner.one([Steps.remove('value.email', 'value.card')], Order))).not.toEqual(expect.arrayContaining(['email']));
    const kept = await runner.one([{ kind: 'keep_only', fields: ['value.orderId', 'value.amount'] }], Order);
    expect(kept).toEqual({ orderId: 'o-1', amount: 12.345 });
  });

  test('TC-TS-05 copy, move / nest and flatten', async ({ ws }) => {
    const out = await new StudioRunner(ws).one(
      [{ kind: 'copy', from: 'value.orderId', to: 'value.ref' }, { kind: 'move', from: 'value.country', to: 'value.address.country' }, { kind: 'flatten', field: 'value.profile' }],
      { ...Order, profile: { name: { first: 'Ada', last: 'Lovelace' }, tags: ['x'] } },
    );
    expect(out.ref).toBe('o-1');
    expect(out.address).toEqual({ country: 'VN' });
    expect(out).not.toHaveProperty('country');
    expect(out.profile).toEqual({ 'name.first': 'Ada', 'name.last': 'Lovelace', tags: ['x'] });
  });

  test('TC-TS-06 set value from constant, field, metadata and formula', async ({ ws }) => {
    const out = await new StudioRunner(ws).one(
      [
        Steps.setConstant('value.source', 'router'),
        { kind: 'set', field: 'value.total', source: { from: 'field', path: 'value.amount' } },
        { kind: 'set', field: 'value.topic', source: { from: 'metadata', name: 'topic' } },
        { kind: 'set', field: 'value.fullName', source: { from: 'formula', expr: 'concat(value.customer.firstName, " ", value.customer.lastName)' } },
        { kind: 'set', field: 'value.gross', source: { from: 'formula', expr: 'round(value.amount * 1.1, 2)' } },
        { kind: 'set', field: 'value.cc', source: { from: 'formula', expr: 'upper(value.country)' } },
      ],
      Order,
    );
    expect(out).toMatchObject({ source: 'router', total: 12.345, fullName: 'Ada Lovelace', gross: 13.58, cc: 'VN' });
    expect(typeof out.topic).toBe('string');
  });

  test('TC-TS-07 convert type, round, string case and format date', async ({ ws }) => {
    const out = await new StudioRunner(ws).one(
      [
        { kind: 'convert_type', field: 'value.status', targetType: 'string' },
        { kind: 'round', field: 'value.amount', decimals: 1 },
        { kind: 'string_case', field: 'value.country', case: 'lower' },
        { kind: 'format_date', field: 'value.at', layout: 'date', timezone: 'UTC' },
      ],
      { ...Order, at: '2026-10-16T23:30:00Z' },
    );
    expect(out).toMatchObject({ status: '1', amount: 12.3, country: 'vn', at: '2026-10-16' });
  });

  test('TC-TS-08 lookup table with default, and default-if-missing', async ({ ws }) => {
    const rows = [{ key: '1', value: 'PAID' }, { key: '2', value: 'REFUNDED' }];
    const runner = new StudioRunner(ws);
    const steps = [{ kind: 'lookup', field: 'value.status', to: 'value.statusName', rows, defaultValue: 'UNKNOWN' }, { kind: 'default_if_missing', field: 'value.currency', value: 'EUR' }];
    const result = await runner.run(steps, [Order, { ...Order, status: 9, currency: 'USD' }]);
    expect(result.values.map((v) => [v.statusName, v.currency])).toEqual([['PAID', 'EUR'], ['UNKNOWN', 'USD']]);
  });

  test('TC-TS-09 if / else with the filter condition builder, and drop-if', async ({ ws }) => {
    const ifElse = { kind: 'if_else', condition: Filter.cond('value.country', 'eq', 'VN'), thenSteps: [Steps.setConstant('value.region', 'APAC')], elseSteps: [Steps.setConstant('value.region', 'OTHER')] };
    const dropIf = { kind: 'drop_if', condition: Filter.cond('value.test', 'eq', true) };
    const result = await new StudioRunner(ws).run([ifElse, dropIf], [Order, { ...Order, country: 'DE' }, { ...Order, test: true }]);
    expect(result.statuses()).toEqual(['passed', 'passed', 'dropped']);
    expect(result.values.map((v) => v.region)).toEqual(['APAC', 'OTHER']);
  });

  test('TC-TS-10 split array, wrap in envelope and unwrap', async ({ ws }) => {
    const runner = new StudioRunner(ws);
    const split = await runner.run([{ kind: 'split_array', field: 'value.items', to: 'value.item' }], [Order]);
    expect(split.values.map((v) => v.item)).toEqual([{ sku: 'A' }, { sku: 'B' }]);
    const wrapped = await runner.one([{ kind: 'wrap', field: 'value.data' }, Steps.setConstant('value.type', 'order')], Order);
    expect(wrapped).toMatchObject({ type: 'order', data: { orderId: 'o-1' } });
    expect(await runner.one([{ kind: 'unwrap', field: 'value.customer' }], Order)).toEqual(Order.customer);
  });

  test('TC-TS-11 Kafka recipes: set key from field and header add / remove / rename / copy', async ({ ws }) => {
    const steps = [
      { kind: 'set_key', source: { from: 'field', path: 'value.orderId' } },
      { kind: 'header_add', header: 'x-src', source: { from: 'constant', value: 'acc' } },
      { kind: 'header_remove', header: 'x-internal' },
      { kind: 'header_rename', header: 'traceId', newHeader: 'trace-id' },
      { kind: 'header_copy_to_payload', header: 'trace-id', field: 'value.traceId' },
    ];
    const result = await new StudioRunner(ws).run(steps, [{ key: 'old', value: Order, headers: [{ key: 'traceId', value: 't-1' }, { key: 'x-internal', value: 'y' }] }]);
    const out = result.outputs[0];
    expect(out.key).toBe('o-1');
    const headers = Object.fromEntries(out.headers.map((h: { key: string; value: string }) => [h.key, h.value]));
    expect(headers).toEqual({ 'x-src': 'acc', 'trace-id': 't-1' });
    expect(out.value.traceId).toBe('t-1');
  });

  test('TC-TS-12 redact by pattern: credit cards and phone numbers', async ({ ws }) => {
    const out = await new StudioRunner(ws).one(
      [{ kind: 'redact_pattern', field: 'value.note', preset: 'credit_card' }, { kind: 'redact_pattern', field: 'value.note2', preset: 'phone' }],
      { note: 'card 4111 1111 1111 1111 ok', note2: 'call +84 912 345 678 now' },
    );
    expect(out.note).not.toContain('4111 1111 1111 1111');
    expect(out.note).toContain('ok');
    expect(out.note2).not.toContain('912 345 678');
  });

  test('TC-TS-18 a disabled step is skipped', async ({ ws }) => {
    const out = await new StudioRunner(ws).one([{ ...Steps.setConstant('value.flag', 1), enabled: false }], Order);
    expect(out).not.toHaveProperty('flag');
  });
});

test.describe('transform studio · JavaScript step', () => {
  test('TC-TS-13 JS contract: event → passed, null → dropped, array → several events', async ({ ws }) => {
    const code = 'function transform(event, ctx) { if (event.value.n === 0) return null; if (event.value.n === 2) return [event, event]; return event; }';
    const result = await new StudioRunner(ws).run([JsSteps.step(code)], [{ n: 0 }, { n: 1 }, { n: 2 }]);
    expect(result.statuses()).toEqual(['dropped', 'passed', 'passed']);
    expect(result.values.map((v) => v.n)).toEqual([1, 2, 2]);
  });

  test('TC-TS-14 a thrown error is reported on the failing event with message and exact line', async ({ ws }) => {
    const code = 'function transform(event, ctx) {\n  if (!event.value.order) {\n    throw new Error("Event has no order");\n  }\n  return event;\n}';
    const result = await new StudioRunner(ws).run([JsSteps.step(code)], [{ order: { items: [] } }, { other: true }]);
    expect(result.statuses()).toEqual(['passed', 'error']);
    const error = JSON.stringify(result.errors());
    expect(error).toContain('Event has no order');
    expect(error).toMatch(/\(3:\d+\)/);
  });

  test('TC-TS-35 a TypeError (property of undefined) is reported on its exact line', async ({ ws }) => {
    const code = 'function transform(event, ctx) {\n  const n = event.value.order.items.length;\n  return event;\n}';
    const result = await new StudioRunner(ws).run([JsSteps.step(code)], [{ other: true }]);
    expect(result.statuses()).toEqual(['error']);
    const error = JSON.stringify(result.errors());
    expect(error).toMatch(/items/);
    expect(error, 'QuickJS reports the function start for property-access TypeErrors').toMatch(/\(2:\d+\)/);
  });

  test('TC-TS-15 ctx helpers behave exactly like the recipe steps', async ({ ws }) => {
    const runner = new StudioRunner(ws);
    const viaJs = await runner.one([JsSteps.step('function transform(event, ctx) { event.value.email = ctx.mask.email(event.value.email); event.value.h = ctx.hash(event.value.orderId); return event; }')], Order);
    const viaRecipe = await runner.one([Steps.mask('value.email', 'email'), { kind: 'copy', from: 'value.orderId', to: 'value.h' }, { kind: 'hash', field: 'value.h' }], Order);
    expect(viaJs.email).toBe(viaRecipe.email);
    expect(viaJs.h).toBe(viaRecipe.h);
  });

  test('TC-TS-16 ctx.log output is returned per event for the console tab', async ({ ws }) => {
    const result = await new StudioRunner(ws).run([JsSteps.step('function transform(event, ctx) { ctx.log("seen " + event.value.n); return event; }')], [{ n: 1 }, { n: 2 }]);
    expect(result.logs().join('\n')).toMatch(/seen 1[\s\S]*seen 2/);
  });

  test('TC-TS-17 determinism: Math.random and Date.now are repeatable between runs', async ({ ws }) => {
    const code = 'function transform(event, ctx) { event.value.r = Math.random(); event.value.t = Date.now(); event.value.n2 = ctx.now(); return event; }';
    const runner = new StudioRunner(ws);
    const events = [{ key: 'k', value: { n: 1 }, timestamp: '2026-10-16T10:00:00Z' }];
    const a = (await runner.run([JsSteps.step(code)], events)).values[0];
    const b = (await runner.run([JsSteps.step(code)], events)).values[0];
    expect(typeof a.r).toBe('number');
    expect(a.r).toBe(b.r);
  });
});

test.describe('transform studio · mapper, export, magic features', () => {
  test('TC-TS-19 Visual Mapper map step: copy input, rename via field source, remove, formula', async ({ ws }) => {
    const map = {
      kind: 'map',
      start: 'copy_input',
      target: {
        fields: {
          customerId: { source: { from: 'field', path: 'value.cust_id' } },
          cust_id: { remove: true },
          card: { remove: true },
          label: { source: { from: 'formula', expr: 'concat(value.orderId, "/", value.country)' } },
          meta: { fields: { route: { source: { from: 'metadata', name: 'route_name' } } } },
        },
      },
    };
    const out = await new StudioRunner(ws).one([map], Order);
    expect(out).toMatchObject({ customerId: 7, label: 'o-1/VN', orderId: 'o-1' });
    expect(out).not.toHaveProperty('cust_id');
    expect(out).not.toHaveProperty('card');
    expect(out.meta).toHaveProperty('route');
  });

  test('TC-TS-20 export recipe steps to an equivalent JS step', async ({ ws, api }) => {
    const steps = [Steps.rename('value.cust_id', 'value.customerId'), Steps.setConstant('value.region', 'APAC')];
    const res = await api.post('/api/v1/assist/export-js', { steps });
    expect(res.status, res.describe()).toBe(200);
    expect(res.body.step.kind).toBe('js');
    const runner = new StudioRunner(ws);
    expect(await runner.one([res.body.step], Order)).toEqual(await runner.one(steps, Order));
  });

  test('TC-TS-21 transform by example infers renames and constants with confidence', async ({ ws, api }) => {
    const res = await api.post('/api/v1/preview/infer-mapping', { input: { value: { cust_id: 7, email: 'a@b.c', drop: 1 } }, desiredOutput: { customerId: 7, email: 'a@b.c', source: 'web' } });
    expect(res.status, res.describe()).toBe(200);
    const kinds = res.body.steps.map((s: { kind: string }) => s.kind);
    expect(kinds).toEqual(expect.arrayContaining(['rename', 'set']));
    expect(res.body.mappings.find((m: { target: string }) => m.target.endsWith('customerId'))).toMatchObject({ source: expect.stringContaining('cust_id') });
    expect(res.body.unmapped.join(',')).toContain('drop');
    const out = await new StudioRunner(ws).one(res.body.steps, { cust_id: 7, email: 'a@b.c', drop: 1 });
    expect(out).toEqual({ customerId: 7, email: 'a@b.c', source: 'web' });
  });

  test('TC-TS-22 AI assist proposes recipe steps as a reviewable diff (only when enabled)', async ({ api }) => {
    const settings = new WorkspaceSettings(api);
    const body = { instruction: 'Hide the email, rename cust_id to customerId', currentSteps: [], fields: ['value.email', 'value.cust_id'], samples: [Order] };
    const refused = await settings.with({ aiAssistEnabled: false }, () => api.post('/api/v1/assist/steps', body));
    expect(refused.status, refused.describe()).toBe(403);
    const res = await settings.with({ aiAssistEnabled: true }, () => api.post('/api/v1/assist/steps', body));
    expect(res.status, res.describe()).toBe(200);
    const kinds = res.body.steps.map((s: { kind: string }) => s.kind);
    expect(kinds).toEqual(expect.arrayContaining(['mask', 'rename']));
    expect(kinds).not.toContain('js');
    expect(res.body.diff.added.length).toBeGreaterThan(0);
  });

  test('TC-TS-23 explain a recipe chain in plain language', async ({ api }) => {
    const steps = [Steps.rename('value.cust_id', 'value.customerId'), Steps.mask('value.email', 'email')];
    const res = await new WorkspaceSettings(api).with({ aiAssistEnabled: true }, () => api.post('/api/v1/assist/explain', { steps }));
    expect(res.status, res.describe()).toBe(200);
    expect(`${res.body.summary} ${res.body.bullets.join(' ')}`).toMatch(/cust_id[\s\S]*customerId|customerId/);
  });
});
