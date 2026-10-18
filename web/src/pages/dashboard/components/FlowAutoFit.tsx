import { useNodesInitialized, useReactFlow, useStore } from '@xyflow/react';
import { useEffect } from 'react';
import { FlowViewport } from '../model/FlowGraph';

/** Re-fits the flow map whenever its pane is resized or its nodes change. */
export function FlowAutoFit({ graphKey }: { graphKey: string }) {
  const { fitView } = useReactFlow();
  const ready = useNodesInitialized();
  const width = useStore((s) => s.width);
  const height = useStore((s) => s.height);
  useEffect(() => {
    if (ready) void fitView(FlowViewport.fit);
  }, [ready, width, height, graphKey, fitView]);
  return null;
}
