import type { Edge, Node } from '@xyflow/react';
import { Format } from '../../../components/ui/Format';
import { FlowNodeType, RouteStatus, RouteStatusMeta, Tone, type FlowEdge, type FlowMap, type FlowNode } from '../../../domain';

/** Custom xyflow element types registered by the flow map. */
export const FlowElementType = { Topic: 'topic', Route: 'route' } as const;

/** Which side of the map a topic node sits on. */
export const FlowSide = { Source: 'source', Destination: 'destination' } as const;
export type FlowSide = (typeof FlowSide)[keyof typeof FlowSide];

export type TopicNodeData = { topic: string; cluster: string; side: FlowSide };
export type RouteEdgeData = {
  routeId: string;
  tone: Tone;
  width: number;
  status: RouteStatus;
  flowSeconds: number;
  hovered: boolean;
};
export type TopicNode = Node<TopicNodeData, typeof FlowElementType.Topic>;
export type RouteEdge = Edge<RouteEdgeData, typeof FlowElementType.Route>;
export type FlowGraphModel = { nodes: TopicNode[]; edges: RouteEdge[]; width: number; height: number };

/** Geometry of the design's 760-wide SVG flow map. */
export const FlowLayout = {
  width: 760,
  rowHeight: 34,
  padding: 10,
  nodeWidth: 200,
  nodeHeight: 26,
  sourceX: 6,
  destinationX: 554,
  keySeparator: '|',
  clusterSeparator: '-',
} as const;

/** Static-diagram viewport: fit edge to edge, allow shrinking on narrow screens, no attribution badge. */
export const FlowViewport = { fit: { padding: 0 }, minZoom: 0.1, pro: { hideAttribution: true } } as const;

/** Line thickness and dash speed by throughput (design `flowMap`). */
export class FlowEdgeStyle {
  static readonly baseWidth = 1.5;
  static readonly widthPerDecade = 1.3;
  static readonly maxSeconds = 2.2;
  static readonly minSeconds = 0.5;
  static readonly secondsPerDecade = 0.5;
  static readonly hoverBoost = 2;

  static readonly toneByStatus: Partial<Record<RouteStatus, Tone>> = {
    [RouteStatus.Error]: Tone.Err,
    [RouteStatus.Paused]: Tone.Off,
  };

  static tone(status: RouteStatus): Tone {
    return FlowEdgeStyle.toneByStatus[status] ?? Tone.Accent;
  }

  static width(status: RouteStatus, eventsPerSec: number): number {
    if (status !== RouteStatus.Running) return FlowEdgeStyle.baseWidth;
    return FlowEdgeStyle.baseWidth + FlowEdgeStyle.decades(eventsPerSec) * FlowEdgeStyle.widthPerDecade;
  }

  static seconds(eventsPerSec: number): number {
    return Math.max(FlowEdgeStyle.minSeconds, FlowEdgeStyle.maxSeconds - FlowEdgeStyle.decades(eventsPerSec) * FlowEdgeStyle.secondsPerDecade);
  }

  private static decades(eventsPerSec: number): number {
    return Math.log10(eventsPerSec + 1);
  }
}

/** Turns the server flow map into topic nodes (sources left, destinations right) and route-branch edges. */
export class FlowGraphBuilder {
  static readonly hint = 'Hover a line to see the route · click to open';

  static build(map: FlowMap, hoveredId: string | null): FlowGraphModel {
    const sources = [...new Set(map.edges.map((e) => e.from))];
    const targets = [...new Set(map.edges.map((e) => e.to))];
    const rows = Math.max(sources.length, targets.length);
    const height = rows * FlowLayout.rowHeight + FlowLayout.padding * 2;
    const sourceOffset = (height - FlowLayout.padding * 2 - sources.length * FlowLayout.rowHeight) / 2;
    const lookup = new FlowNodeLookup(map.nodes);
    const nodes = [
      ...sources.map((id, i) => FlowGraphBuilder.node(lookup, id, FlowSide.Source, FlowLayout.sourceX, FlowGraphBuilder.rowTop(i, sourceOffset))),
      ...targets.map((id, i) => FlowGraphBuilder.node(lookup, id, FlowSide.Destination, FlowLayout.destinationX, FlowGraphBuilder.rowTop(i, 0))),
    ];
    const edges = map.edges.map((e) => FlowGraphBuilder.edge(e, hoveredId));
    return { nodes, edges, width: FlowLayout.width, height };
  }

  /** "Order paid → partner-gateway · 142 ev/s", or the status label when not running. */
  static hoverText(map: FlowMap | null | undefined, hoveredId: string | null): string {
    const edge = map?.edges.find((e) => e.routeId === hoveredId);
    if (!edge) return FlowGraphBuilder.hint;
    const rate = edge.status === RouteStatus.Running ? `${Format.number(edge.eventsPerSec)} ev/s` : RouteStatusMeta[edge.status].label;
    return `${edge.routeName} · ${rate}`;
  }

  private static edge(e: FlowEdge, hoveredId: string | null): RouteEdge {
    const data: RouteEdgeData = {
      routeId: e.routeId,
      tone: FlowEdgeStyle.tone(e.status),
      width: FlowEdgeStyle.width(e.status, e.eventsPerSec),
      status: e.status,
      flowSeconds: FlowEdgeStyle.seconds(e.eventsPerSec),
      hovered: e.routeId === hoveredId,
    };
    return {
      id: e.id,
      type: FlowElementType.Route,
      source: FlowGraphBuilder.nodeId(FlowSide.Source, e.from),
      target: FlowGraphBuilder.nodeId(FlowSide.Destination, e.to),
      data,
    };
  }

  private static node(lookup: FlowNodeLookup, id: string, side: FlowSide, x: number, y: number): TopicNode {
    return {
      id: FlowGraphBuilder.nodeId(side, id),
      type: FlowElementType.Topic,
      position: { x, y },
      data: { topic: lookup.topic(id), cluster: lookup.cluster(id), side },
      draggable: false,
      selectable: false,
      connectable: false,
    };
  }

  private static rowTop(index: number, offset: number): number {
    return FlowLayout.padding + offset + index * FlowLayout.rowHeight + (FlowLayout.rowHeight - FlowLayout.nodeHeight) / 2;
  }

  private static nodeId(side: FlowSide, id: string): string {
    return `${side}${FlowLayout.keySeparator}${id}`;
  }
}

/** Topic labels and their connection's name (the "cluster" shown on each box). */
class FlowNodeLookup {
  private readonly byId: Map<string, FlowNode>;
  private readonly connections: Map<string, string>;

  constructor(nodes: FlowNode[]) {
    this.byId = new Map(nodes.map((n) => [n.id, n]));
    this.connections = new Map(nodes.filter((n) => n.type === FlowNodeType.Connection).map((n) => [n.connectionId, n.label]));
  }

  topic(id: string): string {
    return this.byId.get(id)?.label ?? id;
  }

  cluster(id: string): string {
    const connectionId = this.byId.get(id)?.connectionId ?? '';
    return this.connections.get(connectionId) ?? connectionId.split(FlowLayout.clusterSeparator)[0];
  }
}
