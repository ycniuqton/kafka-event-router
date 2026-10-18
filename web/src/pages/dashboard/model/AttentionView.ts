import { IntentType, RouteRules, type AttentionItem, type Route } from '../../../domain';

/** Resolves the live button label of a "Needs attention" row (Pause flips to Resume once paused). */
export class AttentionView {
  static actionLabel(item: AttentionItem, routes: Route[]): string {
    if (item.intent.type !== IntentType.TogglePause) return item.actionLabel;
    const routeId = item.intent.routeId;
    const route = routes.find((r) => r.id === routeId);
    return route ? RouteRules.pauseLabel(route.status) : item.actionLabel;
  }
}
