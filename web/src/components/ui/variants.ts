/** Variant, size and width constants of the shared UI primitives. */

export const ButtonVariant = { Primary: 'primary', Secondary: 'secondary', Ghost: 'ghost', Danger: 'danger' } as const;
export type ButtonVariant = (typeof ButtonVariant)[keyof typeof ButtonVariant];

/** sm = 26px row actions, md = 30px page actions, lg = 32px top bar / modal actions. */
export const ButtonSize = { Sm: 'sm', Md: 'md', Lg: 'lg' } as const;
export type ButtonSize = (typeof ButtonSize)[keyof typeof ButtonSize];

export const CardPadding = { None: 'none', Sm: 'sm', Md: 'md' } as const;
export type CardPadding = (typeof CardPadding)[keyof typeof CardPadding];

export const CardOverflow = { Clip: 'clip', Scroll: 'scroll', Visible: 'visible' } as const;
export type CardOverflow = (typeof CardOverflow)[keyof typeof CardOverflow];

export const SegmentSize = { Xs: 'xs', Sm: 'sm', Md: 'md' } as const;
export type SegmentSize = (typeof SegmentSize)[keyof typeof SegmentSize];

export const TextSize = { Xs: 'xs', Sm: 'sm', Md: 'md' } as const;
export type TextSize = (typeof TextSize)[keyof typeof TextSize];

export const ControlSize = { Sm: 'sm', Md: 'md', Lg: 'lg' } as const;
export type ControlSize = (typeof ControlSize)[keyof typeof ControlSize];

/** Modal widths from the design: 460 (confirm), 760 (chooser). */
export const ModalWidth = { Sm: 460, Md: 600, Lg: 760 } as const;

export const DrawerWidth = { Default: 460, Wide: 640 } as const;

/** Max content widths used by the design's screens. */
export const PageWidth = { Wide: 1440, Audit: 1240, Library: 1100, Narrow: 760 } as const;

/** Per-line highlight used by diffs and selection. */
export const LineHighlight = { None: 'none', Added: 'added', Changed: 'changed', Selected: 'selectedLine' } as const;
export type LineHighlight = (typeof LineHighlight)[keyof typeof LineHighlight];
