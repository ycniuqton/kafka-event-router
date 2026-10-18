import type { SampleEvent } from '../../../../domain';
import { StudioText } from './StudioText';

/** Which sample the studio shows, and stepping through samples with wrap-around (design `studioEv`). */
export class SampleCursor {
  /** Payload member shown as the event's type when present (e.g. the demo's `eventType`). */
  static readonly typeField = 'eventType';
  readonly samples: SampleEvent[];
  readonly index: number;

  constructor(samples: SampleEvent[], selectedId: number | null) {
    this.samples = samples;
    const found = samples.findIndex((s) => s.id === selectedId);
    this.index = found < 0 ? 0 : found;
  }

  get current(): SampleEvent | undefined {
    return this.samples[this.index];
  }

  get isEmpty(): boolean {
    return this.samples.length === 0;
  }

  get label(): string {
    return StudioText.position(this.index, this.samples.length);
  }

  get eventId(): string {
    const ev = this.current;
    if (!ev) return '';
    const type = (ev.value as Record<string, unknown>)[SampleCursor.typeField];
    return StudioText.eventId(ev.id, typeof type === 'string' ? type : ev.offset >= 0 ? `${ev.topic}@${ev.offset}` : ev.topic);
  }

  previousId(): number | null {
    return this.idAt(this.index - 1);
  }

  nextId(): number | null {
    return this.idAt(this.index + 1);
  }

  private idAt(position: number): number | null {
    const total = this.samples.length;
    if (!total) return null;
    return this.samples[(position + total) % total].id;
  }
}
