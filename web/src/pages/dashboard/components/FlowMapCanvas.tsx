import { ReactFlow, ReactFlowProvider, type EdgeMouseHandler, type EdgeTypes, type NodeTypes } from '@xyflow/react';
import '@xyflow/react/dist/base.css';
import type { CSSProperties } from 'react';
import { FlowElementType, FlowViewport, type FlowGraphModel, type RouteEdge, type TopicNode } from '../model/FlowGraph';
import { FlowAutoFit } from './FlowAutoFit';
import { FlowRouteEdge } from './FlowRouteEdge';
import { FlowTopicNode } from './FlowTopicNode';
import styles from './FlowMapCanvas.module.css';

const nodeTypes: NodeTypes = { [FlowElementType.Topic]: FlowTopicNode };
const edgeTypes: EdgeTypes = { [FlowElementType.Route]: FlowRouteEdge };

export type FlowMapCanvasProps = {
  graph: FlowGraphModel;
  onHover: (routeId: string | null) => void;
  onOpen: (routeId: string) => void;
};

/** Read-only xyflow canvas sized to the graph's aspect ratio (no pan, zoom or drag). */
export function FlowMapCanvas({ graph, onHover, onOpen }: FlowMapCanvasProps) {
  const enter: EdgeMouseHandler<RouteEdge> = (_e, edge) => onHover(edge.data?.routeId ?? null);
  const click: EdgeMouseHandler<RouteEdge> = (_e, edge) => edge.data && onOpen(edge.data.routeId);
  const ratio = { '--flow-ratio': `${graph.width} / ${graph.height}` } as CSSProperties;
  const graphKey = graph.nodes.map((n) => n.id).join();
  return (
    <div className={styles.canvas} style={ratio}>
      <ReactFlowProvider>
        <ReactFlow<TopicNode, RouteEdge>
          nodes={graph.nodes}
          edges={graph.edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onEdgeMouseEnter={enter}
          onEdgeMouseLeave={() => onHover(null)}
          onEdgeClick={click}
          fitView
          fitViewOptions={FlowViewport.fit}
          minZoom={FlowViewport.minZoom}
          nodesDraggable={false}
          nodesConnectable={false}
          nodesFocusable={false}
          edgesFocusable={false}
          elementsSelectable={false}
          panOnDrag={false}
          panOnScroll={false}
          zoomOnScroll={false}
          zoomOnPinch={false}
          zoomOnDoubleClick={false}
          preventScrolling={false}
          proOptions={FlowViewport.pro}
        >
          <FlowAutoFit graphKey={graphKey} />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}
