import { ConnectionStatus, EnvTag, type Connection } from '../domain';
import { MockTime } from './time';

/** Design `CONNS`. */
export const CONNECTIONS: Connection[] = [
  {
    id: 'orders-prod',
    env: EnvTag.Prod,
    bootstrapServers: ['kafka-1.orders.internal:9093', 'kafka-2.orders.internal:9093', 'kafka-3.orders.internal:9093'],
    security: 'SASL/SCRAM-512',
    status: ConnectionStatus.Healthy,
    checkedAt: MockTime.ago(1, 'min'),
    owner: 'Platform',
    topics: [
      { name: 'order-events', partitions: 12, rate: '1.2k/s' },
      { name: 'payment-events', partitions: 6, rate: '310/s' },
      { name: 'finance.refunds', partitions: 3, rate: '0/s' },
      { name: 'ledger.captures', partitions: 6, rate: '86/s' },
    ],
  },
  {
    id: 'partner-kafka',
    env: EnvTag.Prod,
    bootstrapServers: ['broker.partner-gw.net:9094', 'broker2.partner-gw.net:9094'],
    security: 'mTLS',
    status: ConnectionStatus.AuthFailed,
    checkedAt: MockTime.ago(3, 'min'),
    owner: 'Integrations',
    topics: [
      { name: 'orders-public', partitions: 6, rate: '142/s' },
      { name: 'shipments-public', partitions: 3, rate: '37/s' },
    ],
  },
  {
    id: 'analytics-staging',
    env: EnvTag.Staging,
    bootstrapServers: ['an-kafka-1.stg:9092', 'an-kafka-2.stg:9092', 'an-kafka-3.stg:9092'],
    security: 'SASL/PLAIN',
    status: ConnectionStatus.Healthy,
    checkedAt: MockTime.ago(1, 'min'),
    owner: 'Data',
    topics: [
      { name: 'clickstream-web', partitions: 24, rate: '1.6k/s' },
      { name: 'clickstream-app', partitions: 24, rate: '540/s' },
      { name: 'analytics.events', partitions: 24, rate: '2.1k/s' },
      { name: 'user-events', partitions: 6, rate: '19/s' },
      { name: 'analytics.order-totals', partitions: 6, rate: '0/s' },
    ],
  },
  {
    id: 'warehouse-prod',
    env: EnvTag.Prod,
    bootstrapServers: ['kafka-1.wh.internal:9092', 'kafka-2.wh.internal:9092'],
    security: 'OAuth',
    status: ConnectionStatus.Unreachable,
    checkedAt: MockTime.ago(2, 'min'),
    owner: 'Logistics',
    topics: [
      { name: 'inventory-events', partitions: 6, rate: '—' },
      { name: 'warehouse.stock', partitions: 6, rate: '—' },
    ],
  },
  {
    id: 'dev-local',
    env: EnvTag.Dev,
    bootstrapServers: ['localhost:9092'],
    security: 'None',
    status: ConnectionStatus.Healthy,
    checkedAt: MockTime.ago(5, 'min'),
    owner: 'Platform',
    topics: [{ name: 'order-events', partitions: 1, rate: '2/s' }],
  },
];

/** Topic list given to a newly created connection, and default partitions for created topics. */
export const NEW_CONNECTION_TOPICS = [{ name: 'order-events', partitions: 6, rate: '—' }];
export const NEW_TOPIC_DEFAULTS = { partitions: 6, rate: '0/s', retentionDays: 7 } as const;
