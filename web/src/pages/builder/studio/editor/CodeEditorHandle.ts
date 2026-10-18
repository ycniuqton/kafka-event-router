import type { SandboxEntry } from '../../../../domain';

/** Imperative API of the code editor used by the Snippets menu. */
export interface CodeEditorHandle {
  insert(text: string): void;
}

/** An error location shown in the code (1-based line and column, from the sandbox error). */
export type CodeMarker = { line: number; column: number; message: string };

/** Props shared by the Monaco editor and its plain-text fallback. */
export type CodeEditorProps = {
  value: string;
  markers?: CodeMarker[];
  onChange(value: string): void;
  onReady?(handle: CodeEditorHandle | null): void;
  ariaLabel?: string;
  /** Contract the sandbox checks the code against while typing (default `transform`). */
  entry?: SandboxEntry;
};
