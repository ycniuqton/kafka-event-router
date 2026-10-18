import { describe, expect, it } from 'vitest';
import { PushEventType, type PushEvent } from '../domain';
import { PushEventRouter, type PushEffects } from './PushEventRouter';

class RecordingEffects implements PushEffects {
  calls: string[] = [];
  refreshRoute(routeId: string): void {
    this.calls.push(`route:${routeId}`);
  }
  refreshConnections(): void {
    this.calls.push('connections');
  }
  refreshAlerts(): void {
    this.calls.push('alerts');
  }
  announce(message: string): void {
    this.calls.push(`toast:${message}`);
  }
  touch(): void {
    this.calls.push('touch');
  }
}

const frame = (type: PushEvent['type'], payload: Record<string, unknown> = {}): PushEvent => ({ type, at: '2026-10-14T09:00:00Z', payload });

describe('PushEventRouter', () => {
  it('refreshes the route on route state and deployment frames', () => {
    const fx = new RecordingEffects();
    const router = new PushEventRouter(fx);
    router.apply(frame(PushEventType.RouteStateChanged, { routeId: 'r1', status: 'paused' }));
    router.apply(frame(PushEventType.DeploymentStatus, { routeId: 'r2', status: 'active' }));
    expect(fx.calls).toEqual(['route:r1', 'touch', 'route:r2', 'touch']);
  });

  it('updates the bell and toasts when an alert fires, only the bell when it resolves', () => {
    const fx = new RecordingEffects();
    const router = new PushEventRouter(fx);
    router.apply(frame(PushEventType.AlertFired, { routeName: 'orders', ruleName: 'Lag high' }));
    router.apply(frame(PushEventType.AlertResolved, { routeName: 'orders' }));
    expect(fx.calls).toEqual(['alerts', 'toast:Alert firing: orders · Lag high', 'touch', 'alerts', 'touch']);
  });

  it('reloads connections on health changes and everything on resync; ignores hello and unknown frames', () => {
    const fx = new RecordingEffects();
    const router = new PushEventRouter(fx);
    router.apply(frame(PushEventType.ConnectionHealth, { connectionId: 'c1', current: 'unreachable' }));
    router.apply(frame(PushEventType.Hello));
    router.apply({ type: 'nope' as PushEvent['type'], at: '', payload: {} });
    router.apply(frame(PushEventType.Resync));
    expect(fx.calls).toEqual(['connections', 'touch', 'alerts', 'connections', 'touch']);
  });
});
