import type { ComponentType } from 'react';
import { TransformStepKind } from '../../../../../domain';
import { ConvertTypeSettings } from './ConvertTypeSettings';
import { DropIfSettings } from './DropIfSettings';
import { FieldListSettings } from './FieldListSettings';
import { FieldOnlySettings } from './FieldOnlySettings';
import { JsStepSettings } from './JsStepSettings';
import { LookupSettings } from './LookupSettings';
import { LibraryStepSettings } from '../../library/LibraryStepSettings';
import { MaskSettings } from './MaskSettings';
import { RecipeFormSettings } from './RecipeFormSettings';
import { RenameSettings } from './RenameSettings';
import { SetKeySettings } from './SetKeySettings';
import { SetValueSettings } from './SetValueSettings';
import type { StepSettingsProps } from './StepSettingsProps';
import { VisualMapper } from '../../mapper/VisualMapper';

/** Inline settings editor per step kind (RecipeFormRegistry); add an entry to support a new recipe. */
export class StepSettingsRegistry {
  static readonly editors: Record<TransformStepKind, ComponentType<StepSettingsProps>> = {
    [TransformStepKind.Rename]: RenameSettings,
    [TransformStepKind.Copy]: RenameSettings,
    [TransformStepKind.Move]: RenameSettings,
    [TransformStepKind.Remove]: FieldListSettings,
    [TransformStepKind.KeepOnly]: FieldListSettings,
    [TransformStepKind.Mask]: MaskSettings,
    [TransformStepKind.Hash]: FieldOnlySettings,
    [TransformStepKind.ConvertType]: ConvertTypeSettings,
    [TransformStepKind.Set]: SetValueSettings,
    [TransformStepKind.DefaultIfMissing]: SetValueSettings,
    [TransformStepKind.DropIf]: DropIfSettings,
    [TransformStepKind.SetKey]: SetKeySettings,
    [TransformStepKind.Map]: VisualMapper,
    [TransformStepKind.Lookup]: LookupSettings,
    [TransformStepKind.SplitArray]: RecipeFormSettings,
    [TransformStepKind.Wrap]: RecipeFormSettings,
    [TransformStepKind.Unwrap]: RecipeFormSettings,
    [TransformStepKind.HeaderAdd]: RecipeFormSettings,
    [TransformStepKind.HeaderRemove]: RecipeFormSettings,
    [TransformStepKind.HeaderRename]: RecipeFormSettings,
    [TransformStepKind.HeaderToPayload]: RecipeFormSettings,
    [TransformStepKind.HeaderFromPayload]: RecipeFormSettings,
    [TransformStepKind.Library]: LibraryStepSettings,
    [TransformStepKind.Js]: JsStepSettings,
  };
}
