import { AuditObjectType } from '../../../domain';
import { AuditFilter, type AuditFilterState } from './AuditFilter';

/** A named one-click filter of the audit bar; `patch` is the part of the bar state it fixes. */
export type AuditPreset = { id: string; label: string; title: string; patch: Partial<AuditFilterState> };

/** Preset ids (also `?preset=` of the audit page). */
export const AuditPresetId = { DeadLetters: 'dlq' } as const;

/** Query key of the audit page that preselects a preset. */
export const AuditPresetParam = 'preset';

/** Filter presets of the audit page (P3: dead-letter actions — retry, discard, restore, export, views, jobs). */
export class AuditPresets {
  static readonly all: AuditPreset[] = [
    {
      id: AuditPresetId.DeadLetters,
      label: 'Dead-letter actions',
      title: 'Retries, discards, restores, exports, views and background jobs on dead letters',
      patch: { objectType: AuditObjectType.Dlq, action: AuditFilter.any },
    },
  ];

  static byId(id: string | null): AuditPreset | undefined {
    return AuditPresets.all.find((p) => p.id === id);
  }

  static active(filter: AuditFilterState, preset: AuditPreset): boolean {
    return (Object.entries(preset.patch) as [keyof AuditFilterState, unknown][]).every(([k, v]) => filter[k] === v);
  }

  /** Turns the preset on (keeping unrelated filters) or off (back to the initial values of its fields). */
  static toggle(filter: AuditFilterState, preset: AuditPreset): AuditFilterState {
    if (!AuditPresets.active(filter, preset)) return { ...filter, ...preset.patch };
    const reset = Object.fromEntries(Object.keys(preset.patch).map((k) => [k, AuditFilter.initial[k as keyof AuditFilterState]]));
    return { ...filter, ...reset };
  }

  /** Initial bar state of the page, with the `?preset=` preset applied. */
  static initial(id: string | null): AuditFilterState {
    const preset = AuditPresets.byId(id);
    return preset ? { ...AuditFilter.initial, ...preset.patch } : AuditFilter.initial;
  }

  static path(id: string): string {
    return `/audit?${new URLSearchParams({ [AuditPresetParam]: id })}`;
  }
}
