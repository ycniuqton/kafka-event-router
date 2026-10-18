import type { ComponentType } from 'react';
import { RouteDetailTab, type Route } from '../../../domain';

/** What every tab body receives. */
export type DetailTabProps = { route: Route; deadLetters: number };

/** One tab of the route detail page, registered by the feature that owns it. */
export type DetailTabEntry = {
  id: RouteDetailTab;
  order: number;
  label: (props: DetailTabProps) => string;
  Component: ComponentType<DetailTabProps>;
};

/**
 * Tab registry of the route detail shell (routes owns the shell; Overview/Live tail are monitoring's,
 * Dead letters is dead-letters', History and Settings are routes'). Unknown `?tab=` falls back to the first tab.
 */
export class DetailTabs {
  private static readonly entries = new Map<RouteDetailTab, DetailTabEntry>();

  static register(entry: DetailTabEntry): void {
    DetailTabs.entries.set(entry.id, entry);
  }

  static all(): DetailTabEntry[] {
    return [...DetailTabs.entries.values()].sort((a, b) => a.order - b.order);
  }

  static parse(raw: string | null): RouteDetailTab {
    const all = DetailTabs.all();
    return all.find((t) => t.id === raw)?.id ?? all[0]?.id ?? RouteDetailTab.Overview;
  }

  static find(id: RouteDetailTab): DetailTabEntry | undefined {
    return DetailTabs.entries.get(id);
  }

  static items(props: DetailTabProps): { id: RouteDetailTab; label: string }[] {
    return DetailTabs.all().map((t) => ({ id: t.id, label: t.label(props) }));
  }
}

/** Replay defaults and toast copy (spec §10.1: re-read from a chosen time, warn about duplicates). */
export class ReplayPlan {
  static readonly defaultLookbackMs = 3_600_000;
  static readonly inputLength = 16;

  static defaultStart(now: Date): string {
    return new Date(now.getTime() - ReplayPlan.defaultLookbackMs).toISOString().slice(0, ReplayPlan.inputLength);
  }

  static started(start: string, destination: string): string {
    return `Replaying from ${start.replace('T', ' ')} UTC → ${destination}`;
  }
}

/** Toast copy of the dead-letter export stub. */
export class DeadLetterCopy {
  static exported(id: string): string {
    return `Exported ${id} as JSON`;
  }
}
