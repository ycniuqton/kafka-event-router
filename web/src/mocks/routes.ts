import { RouteStatus, TransformMode, type Route } from '../domain';
import { MockTime } from './time';

/** Compact constructor for the seed rows below, mirroring the design's `ROUTES` columns. */
class RouteSeed {
  static make(
    head: Pick<Route, 'id' | 'name' | 'status' | 'owner'>,
    flow: [string, string[], string, string],
    filterSummary: string,
    transformMode: Route['transformMode'],
    metrics: [number, number, number],
    tail: [string, number],
  ): Route {
    const [srcConn, srcTopics, destConn, destTopic] = flow;
    const [eventsPerSec, matchPct, lag] = metrics;
    return {
      ...head,
      source: { connectionId: srcConn, topics: srcTopics },
      destination: { connectionId: destConn, topic: destTopic },
      filterSummary,
      transformMode,
      metrics: { eventsPerSec, matchPct, lag },
      updatedAt: tail[0],
      version: tail[1],
    };
  }
}

const { Running, Paused, Error: Failed, Draft } = RouteStatus;
const { None, Recipe, Js } = TransformMode;
const yesterdayEvening = '2026-10-12T17:30:00.000Z';

/** Design `ROUTES`. */
export const ROUTES: Route[] = [
  RouteSeed.make({ id: 'r1', name: 'Order paid → partner-gateway', status: Running, owner: 'Payments' }, ['orders-prod', ['order-events'], 'partner-kafka', 'orders-public'], 'eventType is OrderPaid +1', Recipe, [142, 7.4, 120], [MockTime.ago(12, 'min'), 4]),
  RouteSeed.make({ id: 'r2', name: 'Refunds → finance', status: Running, owner: 'Finance' }, ['orders-prod', ['order-events'], 'orders-prod', 'finance.refunds'], 'eventType is OrderRefunded', None, [0, 0, 0], [MockTime.ago(3, 'd'), 2]),
  RouteSeed.make({ id: 'r3', name: 'Order totals → analytics', status: Failed, owner: 'Data' }, ['orders-prod', ['order-events'], 'analytics-staging', 'analytics.order-totals'], 'eventType is OrderPaid', Js, [0, 33, 18420], [MockTime.ago(41, 'min'), 7]),
  RouteSeed.make({ id: 'r4', name: 'Clickstream merge', status: Running, owner: 'Data' }, ['analytics-staging', ['clickstream-web', 'clickstream-app'], 'analytics-staging', 'analytics.events'], 'all events', Recipe, [2140, 100, 48210], [MockTime.ago(1, 'd'), 3]),
  RouteSeed.make({ id: 'r5', name: 'Payment captured → ledger', status: Running, owner: 'Finance' }, ['orders-prod', ['payment-events'], 'orders-prod', 'ledger.captures'], 'status is CAPTURED +1', Recipe, [86, 41, 40], [MockTime.ago(5, 'd'), 5]),
  RouteSeed.make({ id: 'r6', name: 'Stock changes → warehouse', status: Paused, owner: 'Logistics' }, ['warehouse-prod', ['inventory-events'], 'warehouse-prod', 'warehouse.stock'], 'delta ≠ 0', None, [0, 58, 2310], [MockTime.ago(2, 'h'), 2]),
  RouteSeed.make({ id: 'r7', name: 'Shipment updates → partner', status: Running, owner: 'Integrations' }, ['orders-prod', ['order-events'], 'partner-kafka', 'shipments-public'], 'eventType is OrderShipped', Recipe, [37, 12, 210], [MockTime.ago(6, 'h'), 9]),
  RouteSeed.make({ id: 'r8', name: 'Fraud signals', status: Draft, owner: 'Risk' }, ['orders-prod', ['payment-events'], 'orders-prod', 'risk.signals'], 'score > 0.8', Js, [0, 0, 0], [yesterdayEvening, 0]),
  RouteSeed.make({ id: 'r9', name: 'User signups → CRM', status: Running, owner: 'Growth' }, ['analytics-staging', ['user-events'], 'analytics-staging', 'crm.signups'], 'type is signup', Recipe, [12, 64, 0], [MockTime.ago(2, 'w'), 3]),
  RouteSeed.make({ id: 'r10', name: 'Cart abandonment', status: Paused, owner: 'Growth' }, ['analytics-staging', ['clickstream-web'], 'analytics-staging', 'marketing.carts'], 'event is cart_idle +2', Js, [0, 3, 0], [MockTime.ago(1, 'w'), 6]),
];

/** Numbers the mock backend assigns on deploy/resume (design `deploy`, `togglePause`). */
export const MOCK_ROUTE_DEFAULTS = {
  newRouteOwner: 'Platform',
  newRouteTopic: 'new-topic',
  deployedEventsPerSec: 24,
  newRouteMatchPct: 33,
  resumedEventsPerSec: 30,
  idModulo: 100_000,
  copyIdModulo: 1_000,
  replicas: 2,
} as const;
