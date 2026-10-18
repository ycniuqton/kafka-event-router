import type { RouteStatus } from './route';

/** Node kinds of the server flow map. */
export const FlowNodeType = { Connection: 'connection', Topic: 'topic' } as const;
export type FlowNodeType = (typeof FlowNodeType)[keyof typeof FlowNodeType];

export type FlowNode = { id: string; type: FlowNodeType; label: string; connectionId: string; envTag?: string };
/** One route branch from a source topic node to a destination topic node. */
export type FlowEdge = {
  id: string;
  routeId: string;
  routeName: string;
  teamName: string;
  branch: string;
  from: string;
  to: string;
  eventsPerSec: number;
  status: RouteStatus;
};
export type FlowMap = { nodes: FlowNode[]; edges: FlowEdge[]; metricsAvailable: boolean };

/** `GET /flow-map` filters; empty strings mean "all". */
export type FlowMapQuery = { connection: string; team: string; tag: string };
