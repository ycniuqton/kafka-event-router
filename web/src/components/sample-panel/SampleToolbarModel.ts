import { PreviewLimit, SampleMode, ValueFormat } from '../../domain';

/** Load modes offered by the toolbar (demo samples are P3). */
export const ToolbarMode = { Latest: SampleMode.Latest, TimeRange: SampleMode.TimeRange, Upload: SampleMode.Upload } as const;
export type ToolbarMode = (typeof ToolbarMode)[keyof typeof ToolbarMode];

/** Options and validation of the sample toolbar. */
export class SampleToolbarModel {
  static readonly counts = [PreviewLimit.PeekCount, PreviewLimit.DefaultCount, 100, 500, PreviewLimit.MaxEvents] as const;
  static readonly modes: { value: ToolbarMode; label: string }[] = [
    { value: ToolbarMode.Latest, label: 'Latest' },
    { value: ToolbarMode.TimeRange, label: 'Time range' },
    { value: ToolbarMode.Upload, label: 'Upload' },
  ];
  static readonly defaultFormat = ValueFormat.Json;
  static readonly uploadAccept = '.json,.ndjson,.jsonl,application/json';
  static readonly bytesPerMb = 1024 * 1024;
  static readonly hourMs = 3_600_000;
  static readonly minuteMs = 60_000;
  static readonly localInputLength = 16;

  static tooLarge(bytes: number): string | null {
    if (bytes <= PreviewLimit.MaxUploadBytes) return null;
    return `The file is larger than ${PreviewLimit.MaxUploadBytes / SampleToolbarModel.bytesPerMb} MB.`;
  }

  /** `datetime-local` value (local time, minutes) of an instant. */
  static localInput(at: Date): string {
    const shifted = new Date(at.getTime() - at.getTimezoneOffset() * SampleToolbarModel.minuteMs);
    return shifted.toISOString().slice(0, SampleToolbarModel.localInputLength);
  }

  /** RFC 3339 instant of a `datetime-local` value, or null when empty/invalid. */
  static instant(local: string): string | null {
    const at = new Date(local);
    return Number.isNaN(at.getTime()) ? null : at.toISOString();
  }

  static rangeError(from: string | null, to: string | null): string | null {
    if (!from || !to) return 'Pick both a start and an end time.';
    return from < to ? null : 'The start must be before the end.';
  }
}
