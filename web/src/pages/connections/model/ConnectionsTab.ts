/** Tabs of the Connections screen: Kafka clusters (default) and lookup sources (enrichment targets). */
export const ConnectionsTab = { Kafka: 'kafka', Lookups: 'lookups' } as const;
export type ConnectionsTab = (typeof ConnectionsTab)[keyof typeof ConnectionsTab];

export const ConnectionsTabs = [
  { id: ConnectionsTab.Kafka, label: 'Kafka clusters' },
  { id: ConnectionsTab.Lookups, label: 'Lookup sources' },
];
