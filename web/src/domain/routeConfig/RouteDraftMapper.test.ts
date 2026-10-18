import { describe, expect, it } from 'vitest';
import type { RouteConfig } from '../../api/generated';
import { DestinationOptions, StartPosition } from '../builder';
import { FilterCombinator, FilterMode, FilterOp } from '../filter';
import { RouteStatus } from '../route';
import { TransformStepKind } from '../transform';
import { FilterTreeModel } from './FilterTreeModel';
import { OperatorCatalog } from './OperatorCatalog';
import { RouteDraftMapper } from './RouteDraftMapper';

const doc: RouteConfig = {
  apiVersion: 'router/v1',
  name: 'orders',
  source: { connectionId: 'c1', topics: ['order-events', 'payment-events'], format: { type: 'json' }, consumerGroup: 'router.orders', startFrom: { mode: 'earliest' } },
  branches: [
    {
      id: 'main',
      filter: {
        type: 'and',
        children: [
          { type: 'cond', field: 'value.eventType', op: 'eq', value: 'OrderPaid' },
          { type: 'cond', field: 'value.amount', op: 'gt', value: 100 },
          { type: 'cond', field: 'value.country', op: 'in', values: ['DE', 'FR'] },
        ],
      },
      transform: {
        steps: [
          { kind: 'rename', from: 'value.cust_id', to: 'value.customerId' },
          { kind: 'mask', field: 'value.email', style: 'email' },
          { kind: 'set_key', source: { from: 'field', path: 'value.orderId' } },
          { kind: 'js', code: 'function transform(e) { return e; }', enabled: false },
        ],
      },
      destination: { connectionId: 'c2', topic: 'orders-public', partitioning: 'round_robin', key: { mode: 'none' }, headers: { mode: 'keep_selected', keep: ['traceId'] } },
    },
  ],
  errorPolicy: { onError: 'skip', maxRetries: 3 },
  limits: { maxInFlight: 500 },
};

const meta = { routeId: 'r1', name: 'orders', status: RouteStatus.Running };

describe('RouteDraftMapper', () => {
  it('reads a document into the builder draft', () => {
    const { draft } = RouteDraftMapper.fromDocument(doc, meta);
    expect(draft.source).toMatchObject({ connectionId: 'c1', topics: ['order-events', 'payment-events'], start: StartPosition.Earliest, multi: true });
    expect(draft.filter.rows).toEqual([
      { field: 'value.eventType', op: FilterOp.Equals, value: 'OrderPaid' },
      { field: 'value.amount', op: FilterOp.GreaterThan, value: '100' },
      { field: 'value.country', op: FilterOp.InList, value: 'DE, FR' },
    ]);
    expect(draft.steps.map((s) => [s.kind, s.field, s.argument, s.enabled])).toEqual([
      [TransformStepKind.Rename, 'cust_id', 'customerId', true],
      [TransformStepKind.Mask, 'email', '', true],
      [TransformStepKind.SetKey, 'orderId', '', true],
      [TransformStepKind.Js, '', '', false],
    ]);
    expect(draft.destination).toMatchObject({
      key: DestinationOptions.key[2],
      partitioning: DestinationOptions.partitioning[2],
      headers: DestinationOptions.headers[1],
      onError: DestinationOptions.onError[0],
    });
  });

  it('round-trips a document without losing server-side members', () => {
    const { draft, snapshot } = RouteDraftMapper.fromDocument(doc, meta);
    const out = RouteDraftMapper.toDocument(draft, snapshot);
    expect(out.source.consumerGroup).toBe('router.orders');
    expect(out.limits).toEqual({ maxInFlight: 500 });
    expect(out.errorPolicy).toEqual({ onError: 'skip', maxRetries: 3 });
    expect(out.branches[0].filter).toEqual(doc.branches[0].filter);
    expect(out.branches[0].transform?.steps).toEqual(doc.branches[0].transform?.steps);
    expect(out.branches[0].destination.headers).toEqual({ mode: 'keep_selected', keep: ['traceId'] });
  });

  it('keeps filters the builder cannot show until the user changes them', () => {
    const odd: RouteConfig = { ...doc, branches: [{ ...doc.branches[0], filter: { type: 'cond', field: 'value.a', op: 'regex', value: '^x' } }] };
    const { draft, snapshot } = RouteDraftMapper.fromDocument(odd, meta);
    expect(draft.filter.mode).toBe(FilterMode.Expression);
    expect(RouteDraftMapper.toDocument(draft, snapshot).branches[0].filter).toEqual(odd.branches[0].filter);
    draft.filter = { ...draft.filter, expression: 'event.value.a > 1' };
    expect(RouteDraftMapper.toDocument(draft, snapshot).branches[0].filter).toEqual({ type: 'expr', code: 'event.value.a > 1' });
  });

  it('builds a fresh document for a new draft', () => {
    const { draft } = RouteDraftMapper.fromDocument(doc, meta);
    draft.filter = { combinator: FilterCombinator.And, rows: [], mode: FilterMode.Visual, expression: '' };
    draft.steps = [];
    const out = RouteDraftMapper.toDocument(draft);
    expect(out.apiVersion).toBe('router/v1');
    expect(out.branches[0].filter).toBeUndefined();
    expect(out.branches[0].transform).toBeUndefined();
    expect(out.source.format).toEqual({ type: 'json' });
  });
});

describe('FilterTreeModel and OperatorCatalog', () => {
  it('maps every visual operator to the engine and back', () => {
    for (const op of Object.values(FilterOp)) {
      const value = OperatorCatalog.engine(op).shape === 'number' ? '5' : 'a';
      const node = FilterTreeModel.cond({ field: 'value.x', op, value });
      const back = FilterTreeModel.fromNode(node);
      expect(back.exact).toBe(true);
      expect(back.group.rows[0].op).toBe(op);
    }
  });

  it('uses or-groups, expressions and the match-all case', () => {
    const or = FilterTreeModel.toNode({
      combinator: FilterCombinator.Or,
      rows: [
        { field: 'value.a', op: FilterOp.Exists, value: '' },
        { field: 'value.b', op: FilterOp.NumNotEquals, value: '2' },
      ],
      mode: FilterMode.Visual,
      expression: '',
    });
    expect(or).toEqual({ type: 'or', children: [{ type: 'cond', field: 'value.a', op: 'exists' }, { type: 'cond', field: 'value.b', op: 'ne', value: 2 }] });
    expect(FilterTreeModel.fromNode(or).group.combinator).toBe(FilterCombinator.Or);
    expect(FilterTreeModel.fromNode({ type: 'expr', code: 'true' }).group).toMatchObject({ mode: FilterMode.Expression, expression: 'true' });
    expect(FilterTreeModel.fromNode({ type: 'not', child: { type: 'cond', field: 'value.a', op: 'exists' } }).exact).toBe(false);
    expect(FilterTreeModel.toNode({ combinator: FilterCombinator.And, rows: [], mode: FilterMode.Visual, expression: '' })).toBeUndefined();
  });
});

describe('nested filters, literals and draft extras', () => {
  const nested: RouteConfig['branches'][0]['filter'] = {
    type: 'and',
    children: [
      { type: 'cond', field: 'value.keep', op: 'eq', value: true },
      { type: 'or', children: [{ type: 'cond', field: 'value.a', op: 'eq', value: 'x' }, { type: 'cond', field: 'value.n', op: 'gt', value: 3 }] },
    ],
  };

  it('reads nested and/or groups and boolean literals visually and writes them back unchanged', () => {
    const read = FilterTreeModel.fromNode(nested ?? undefined);
    expect(read.exact).toBe(true);
    expect(read.group.rows).toEqual([{ field: 'value.keep', op: FilterOp.Equals, value: 'true', literal: true }]);
    expect(read.group.groups?.[0].combinator).toBe(FilterCombinator.Or);
    expect(FilterTreeModel.toNode(read.group)).toEqual(nested);
  });

  it('gives JSON Pointers of every condition following the collapse rule', () => {
    const read = FilterTreeModel.fromNode(nested ?? undefined).group;
    const pointers = FilterTreeModel.pointers(read);
    expect(pointers.rows).toEqual(['/branches/0/filter/children/0']);
    expect(pointers.groups[0].rows).toEqual(['/branches/0/filter/children/1/children/0', '/branches/0/filter/children/1/children/1']);
    const single = FilterTreeModel.pointers({ ...read, groups: [] });
    expect(single.rows).toEqual(['/branches/0/filter']);
  });

  it('maps consumer group, start time, key field, headers, Avro subject and createTopic', () => {
    const { draft, snapshot } = RouteDraftMapper.fromDocument(doc, meta);
    expect(draft.source.consumerGroup).toBe('router.orders');
    draft.source = { ...draft.source, start: StartPosition.DateTime, startAt: '2026-10-14T06:00:00.000Z', consumerGroup: 'custom.group' };
    draft.destination = {
      ...draft.destination,
      key: DestinationOptions.key[1],
      keyField: 'value.orderId',
      addHeaders: [{ key: 'x-src', value: 'router' }, { key: ' ', value: 'dropped' }],
      format: DestinationOptions.format[2],
      schemaSubject: 'orders-value',
      createTopic: { partitions: 6 },
    };
    const out = RouteDraftMapper.toDocument(draft, snapshot);
    expect(out.source.consumerGroup).toBe('custom.group');
    expect(out.source.startFrom).toEqual({ mode: 'timestamp', at: '2026-10-14T06:00:00.000Z' });
    const dest = out.branches[0].destination;
    expect(dest.key).toEqual({ mode: 'field', field: 'value.orderId' });
    expect(dest.headers?.add).toEqual([{ key: 'x-src', value: 'router' }]);
    expect(dest.format).toEqual({ type: 'avro', subject: 'orders-value' });
    expect(dest.createTopic).toEqual({ partitions: 6 });
    draft.source = { ...draft.source, consumerGroup: '' };
    draft.destination = { ...draft.destination, createTopic: null };
    const cleared = RouteDraftMapper.toDocument(draft, snapshot);
    expect(cleared.source.consumerGroup).toBeUndefined();
    expect(cleared.branches[0].destination.createTopic).toBeUndefined();
  });
});
