/** Certificate / key files accepted by the PEM drop zones (spec §5.2: `.pem` drag-and-drop). */
export class PemFiles {
  static readonly extensions = ['.pem', '.crt', '.key'] as const;
  static readonly accept = PemFiles.extensions.join(',');
  static readonly beginMarker = '-----BEGIN ';
  static readonly notPem = 'This file is not PEM text (expected -----BEGIN …-----).';
  static readonly lineBreak = '\n';

  static isPemName(name: string): boolean {
    const lower = name.toLowerCase();
    return PemFiles.extensions.some((ext) => lower.endsWith(ext));
  }

  static isPemText(text: string): boolean {
    return text.includes(PemFiles.beginMarker);
  }

  /** Reads the first file as PEM text; rejects other files with a plain-language message. */
  static async read(files: File[]): Promise<{ text: string; name: string } | { error: string }> {
    const file = files[0];
    if (!file) return { error: PemFiles.notPem };
    const text = await file.text();
    if (!PemFiles.isPemText(text)) return { error: `${file.name}: ${PemFiles.notPem}` };
    return { text: text.trim(), name: file.name };
  }

  /** Short summary of loaded PEM text, e.g. "CERTIFICATE · 21 lines". */
  static summary(text: string): string {
    const kind = text.slice(text.indexOf(PemFiles.beginMarker) + PemFiles.beginMarker.length).split('-----')[0];
    return `${kind} · ${text.split(PemFiles.lineBreak).length} lines`;
  }
}
