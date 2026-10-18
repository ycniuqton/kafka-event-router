import { PanelValueActionKind } from '../../domain';

/** Labels of the value actions offered by the panel (owners: routes filter, transform-studio steps). */
export class ValueActionLabels {
  static readonly labels: Record<PanelValueActionKind, (path: string, value: unknown) => string> = {
    [PanelValueActionKind.KeepOnly]: (path, value) => `Keep only events where ${path} = ${JSON.stringify(value)}`,
    [PanelValueActionKind.Mask]: (path) => `Mask ${path}`,
    [PanelValueActionKind.Rename]: (path) => `Rename ${path}…`,
    [PanelValueActionKind.Remove]: (path) => `Remove ${path}`,
    [PanelValueActionKind.UseAsKey]: (path) => `Use ${path} as key`,
  };
}
