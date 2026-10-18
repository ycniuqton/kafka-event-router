import type { BuilderCommands } from '../model/BuilderCommands';
import type { BuilderStepContext } from '../model/BuilderStepContext';

/** Props every step editor of the centre zone receives. */
export type StepEditorProps = { ctx: BuilderStepContext; commands: BuilderCommands };
