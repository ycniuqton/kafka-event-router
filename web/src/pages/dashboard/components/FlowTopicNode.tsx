import { Handle, Position, type NodeProps } from '@xyflow/react';
import { FlowSide, type TopicNode } from '../model/FlowGraph';
import styles from './FlowTopicNode.module.css';

/** Topic box on the flow map: topic name left, cluster prefix right; one hidden handle facing the routes. */
export function FlowTopicNode({ data }: NodeProps<TopicNode>) {
  const isSource = data.side === FlowSide.Source;
  return (
    <div className={styles.node} title={`${data.cluster} · ${data.topic}`}>
      <span className={styles.topic}>{data.topic}</span>
      <span className={styles.cluster}>{data.cluster}</span>
      <Handle
        className={styles.handle}
        type={isSource ? 'source' : 'target'}
        position={isSource ? Position.Right : Position.Left}
        isConnectable={false}
      />
    </div>
  );
}
