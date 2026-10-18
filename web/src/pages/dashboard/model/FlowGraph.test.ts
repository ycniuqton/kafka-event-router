import { describe, expect, it } from 'vitest';
import { FlowNodeType, RouteStatus, type FlowMap } from '../../../domain';
import { FlowEdgeStyle, FlowGraphBuilder } from './FlowGraph';

const map: FlowMap = {
  metricsAvailable: true,
  nodes: [
    { id: 'conn:c1', type: FlowNodeType.Connection, label: 'prod-orders', connectionId: 'c1' },
    { id: 'topic:c1:in', type: FlowNodeType.Topic, label: 'in', connectionId: 'c1' },
    { id: 'topic:c1:out', type: FlowNodeType.Topic, label: 'out', connectionId: 'c1' },
  ],
  edges: [
    { id: 'r1:main:in', routeId: 'r1', routeName: 'Orders', teamName: 't', branch: 'main', from: 'topic:c1:in', to: 'topic:c1:out', eventsPerSec: 142, status: RouteStatus.Running },
    { id: 'r2:main:in', routeId: 'r2', routeName: 'Refunds', teamName: 't', branch: 'main', from: 'topic:c1:in', to: 'topic:c1:out', eventsPerSec: 0, status: RouteStatus.Error },
  ],
};

describe('FlowGraphBuilder (server flow map)', () => {
  it('draws source topics left, destinations right, labelled with the connection name', () => {
    const g = FlowGraphBuilder.build(map, 'r1');
    expect(g.nodes.map((n) => n.id)).toEqual(['source|topic:c1:in', 'destination|topic:c1:out']);
    expect(g.nodes[0].data).toMatchObject({ topic: 'in', cluster: 'prod-orders' });
    expect(g.edges[0]).toMatchObject({ source: 'source|topic:c1:in', target: 'destination|topic:c1:out' });
    expect(g.edges[0].data).toMatchObject({ hovered: true, width: FlowEdgeStyle.width(RouteStatus.Running, 142) });
    expect(g.edges[1].data).toMatchObject({ tone: 'err', width: FlowEdgeStyle.baseWidth });
  });

  it('describes the hovered route with its rate or status', () => {
    expect(FlowGraphBuilder.hoverText(map, 'r1')).toBe('Orders · 142 ev/s');
    expect(FlowGraphBuilder.hoverText(map, 'r2')).toBe('Refunds · Error');
    expect(FlowGraphBuilder.hoverText(undefined, null)).toBe(FlowGraphBuilder.hint);
  });
});
