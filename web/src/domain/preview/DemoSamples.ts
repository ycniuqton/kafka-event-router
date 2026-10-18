import { ClusterKind, type Connection } from '../connection';
import { SampleMode, type LoadSamplesRequest } from './types';

/**
 * Demo sandbox samples (FR-9): a demo connection reading only `order-events` loads synthetic order events
 * from the preview service's demo source instead of reading Kafka, so the panel fills with zero setup.
 */
export class DemoSamples {
  static readonly topic = 'order-events';
  static readonly notice = 'Demo data: synthetic order events (no Kafka read).';

  /** True when the source is the demo sandbox's order topic. */
  static applies(connection: Pick<Connection, 'kind'> | undefined, topics: readonly string[]): boolean {
    return connection?.kind === ClusterKind.Demo && topics.length === 1 && topics[0] === DemoSamples.topic;
  }

  /** True when a load request is served by the demo source. */
  static isDemo(request: LoadSamplesRequest | null): boolean {
    return request?.mode === SampleMode.Demo;
  }
}
