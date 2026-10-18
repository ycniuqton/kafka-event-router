/** UI-only modes of the Transform Studio (never stored in the route draft). */

/** How the Input column renders the current sample. */
export const InputView = { Tree: 'tree', Table: 'table' } as const;
export type InputView = (typeof InputView)[keyof typeof InputView];

/** Pane shown in the shared side column below the wide breakpoint (1440px). */
export const SidePane = { Input: 'input', Output: 'output' } as const;
export type SidePane = (typeof SidePane)[keyof typeof SidePane];

/** Tabs of a JavaScript step: the code editor or the per-event ctx.log console. */
export const JsPanelTab = { Code: 'code', Console: 'console' } as const;
export type JsPanelTab = (typeof JsPanelTab)[keyof typeof JsPanelTab];

/** Diff state of one output line. */
export const OutputMark = { None: 'none', Added: 'added', Changed: 'changed' } as const;
export type OutputMark = (typeof OutputMark)[keyof typeof OutputMark];

/** Border state of a step card: failing on the current event, selected, or idle. */
export const StepCardState = { Error: 'error', Selected: 'selected', Idle: 'idle' } as const;
export type StepCardState = (typeof StepCardState)[keyof typeof StepCardState];

/** Resolves a card's state; an error on the current event wins over selection. */
export class StepCardStateOf {
  static resolve(failing: boolean, selected: boolean): StepCardState {
    if (failing) return StepCardState.Error;
    return selected ? StepCardState.Selected : StepCardState.Idle;
  }
}
