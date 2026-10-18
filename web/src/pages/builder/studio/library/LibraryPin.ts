import { LibraryRules, LookupMode, TransformStepKind, type TransformStep } from '../../../../domain';

/** Texts of library pins on step cards. */
export class LibraryPinText {
  static readonly update = 'Update';
  static available(version: number): string {
    return `Update available: ${LibraryRules.versionLabel(version)}`;
  }
  static updated(name: string, version: number): string {
    return `Updated ${LibraryRules.pinLabel(name, version)}`;
  }
}

/** The pinned library object of a step: a library transform, or the named table of a lookup step. */
export class LibraryPin {
  static of(step: TransformStep): { name: string; version: number; patch(version: number): Partial<TransformStep> } | null {
    if (step.kind === TransformStepKind.Library && step.libraryRef && step.libraryVersion) {
      return { name: step.libraryRef, version: step.libraryVersion, patch: (libraryVersion) => ({ libraryVersion }) };
    }
    if (step.kind === TransformStepKind.Lookup && step.lookupMode === LookupMode.Table && step.lookupRef && step.lookupVersion) {
      return { name: step.lookupRef, version: step.lookupVersion, patch: (lookupVersion) => ({ lookupVersion }) };
    }
    return null;
  }
}

/** Texts of the library pickers. */
export class LibraryPickerText {
  static readonly choose = 'choose…';
  static readonly object = 'Library object';
  static readonly version = 'Version';
  static readonly empty = 'The library has nothing of this kind yet.';
}
