import { ProjectedState, Tone } from '../../domain';

/** Icon, tone and label of each per-event status (spec §9, colours per §12). */
export const EventStatusMeta: Record<ProjectedState, { icon: string; tone: Tone; label: string }> = {
  [ProjectedState.Loaded]: { icon: '•', tone: Tone.Muted, label: 'loaded' },
  [ProjectedState.Passed]: { icon: '✓', tone: Tone.Ok, label: 'passed' },
  [ProjectedState.Filtered]: { icon: '⊘', tone: Tone.Off, label: 'filtered' },
  [ProjectedState.Error]: { icon: '⚠', tone: Tone.Err, label: 'error' },
  [ProjectedState.Dropped]: { icon: '⊖', tone: Tone.Warn, label: 'dropped' },
};
