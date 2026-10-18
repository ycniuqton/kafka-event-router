import type { JsonObject } from './json';

/** A Kafka record as seen by filters and transforms: key, headers and JSON value. */
export type KafkaEvent<V extends JsonObject = JsonObject> = {
  key: string;
  headers: Record<string, string>;
  value: V;
};

export type OrderItem = { sku: string; qty: number; price: number };

/** Payload of the demo `order-events` topic. */
export type OrderEventValue = {
  eventType: string;
  orderId: string;
  cust_id: string;
  email: string;
  amount: number;
  currency: string;
  country: string;
  items?: OrderItem[];
};

/** A sampled record with its Kafka coordinates, used by previews, tail and dead letters. */
export type SampleEvent = KafkaEvent<OrderEventValue> & {
  id: number;
  topic: string;
  partition: number;
  offset: number;
  timestamp: string;
};
