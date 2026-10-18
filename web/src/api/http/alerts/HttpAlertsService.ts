import {
  AlertRuleRules,
  AlertState,
  AlertStateTone,
  type Alert,
  type AlertFeed,
  type AlertRule,
  type AlertRuleSave,
  type AttentionItem,
  type ChannelTestResult,
} from '../../../domain';
import { AlertsService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { AlertsEndpoint, AlertsFeedLimit, AlertsParam, type AlertFeedDto, type AlertFeedItemDto, type AttentionDto, type AttentionItemDto } from './AlertDtos';

/** `services.alerts` over `/api/v1`: needs-attention rows, the bell feed (`/alerts`) and alert rules (`/alert-rules`). */
export class HttpAlertsService extends AlertsService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(): Promise<Alert[]> {
    return (await this.feed()).items;
  }

  async attention(): Promise<AttentionItem[]> {
    const dto = await this.context.api.get<AttentionDto>(AlertsEndpoint.attention);
    return dto.items.map((item) => AttentionMapper.item(item));
  }

  async feed(): Promise<AlertFeed> {
    const dto = await this.context.api.get<AlertFeedDto>(AlertsEndpoint.feed, { [AlertsParam.limit]: AlertsFeedLimit }, { silent: true });
    return { items: dto.items.map((i) => AlertFeedMapper.item(i)), unread: dto.unread };
  }

  async ack(id: string): Promise<void> {
    await this.context.api.post<void>(AlertsEndpoint.ack(id));
  }

  async rules(routeId: string): Promise<AlertRule[]> {
    const dto = await this.context.api.get<{ items: AlertRule[] }>(AlertsEndpoint.rules, { [AlertsParam.routeId]: routeId });
    return dto.items;
  }

  saveRule({ draft, existing }: AlertRuleSave): Promise<AlertRule> {
    if (!existing) return this.context.api.post<AlertRule>(AlertsEndpoint.rules, draft, { silent: true });
    const { routeId: _routeId, ...patch } = draft;
    return this.context.api.patch<AlertRule>(AlertsEndpoint.rule(existing.id), patch, { ifMatch: `"${existing.version}"`, silent: true });
  }

  async deleteRule(id: string): Promise<void> {
    await this.context.api.delete(AlertsEndpoint.rule(id));
  }

  async testRule(id: string): Promise<ChannelTestResult[]> {
    const dto = await this.context.api.post<{ results: ChannelTestResult[] }>(AlertsEndpoint.test(id));
    return dto.results;
  }
}

/** Maps attention rows onto the dashboard card shape. */
export class AttentionMapper {
  static item(dto: AttentionItemDto): AttentionItem {
    return { id: dto.id, tone: dto.tone, kind: dto.kind, title: dto.title, detail: dto.detail, actionLabel: dto.action.label, intent: dto.action.intent };
  }
}

/** Maps bell feed rows: "<route> · <rule>" and the rule condition with the observed value. */
export class AlertFeedMapper {
  static readonly noData = 'no data';

  static item(dto: AlertFeedItemDto): Alert {
    const value = dto.value === null ? AlertFeedMapper.noData : dto.value.toLocaleString('en-US');
    const condition = AlertRuleRules.summary({ kind: dto.kind, threshold: dto.threshold, windowMinutes: dto.windowMinutes });
    const resolved = dto.state === AlertState.Resolved ? ' · resolved' : '';
    return {
      id: String(dto.id),
      tone: AlertStateTone[dto.state],
      title: `${dto.routeName} · ${dto.ruleName}`,
      detail: `${condition} · now ${value}${resolved}`,
      intent: dto.intent,
      state: dto.state,
      acked: dto.acked,
      at: dto.resolvedAt ?? dto.firedAt,
    };
  }
}
