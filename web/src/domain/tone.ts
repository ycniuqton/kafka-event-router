/**
 * Semantic colour tones shared by every status indicator.
 * A tone maps to a CSS token via the `data-tone` attribute (see styles/tokens.css).
 */
export const Tone = {
  Ok: 'ok',
  Warn: 'warn',
  Err: 'err',
  Off: 'off',
  Info: 'info',
  Accent: 'acc',
  Prod: 'prod',
  Muted: 'muted',
  Code: 'code',
  Text: 'text',
  TextSecondary: 'text2',
} as const;
export type Tone = (typeof Tone)[keyof typeof Tone];
