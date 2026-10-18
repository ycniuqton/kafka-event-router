/* Reads spliced library step ids from engine traces (`<stepId>/<ref>@<version>/<childStepId>`, catalogue `library`). */
import { engineCatalog } from "./engineCatalog";

/** One trace step id produced by a library step expansion. */
export interface SplicedStepId {
  stepId: string;
  ref: string;
  version: number;
  childStepId: string;
}

/** Parses and matches trace step ids of library steps so Studio can attribute them to the route step. */
export class LibraryStepIds {
  private static readonly idSeparator = engineCatalog.library.stepIdSeparator;
  private static readonly versionSeparator = engineCatalog.library.versionSeparator;
  private static readonly pinPattern = new RegExp(
    `^(.+)${LibraryStepIds.escape(LibraryStepIds.versionSeparator)}([1-9][0-9]*)$`,
  );

  /** Splits the outermost library pin of a trace id; null when the id is not a spliced library step. */
  static parse(traceStepId: string): SplicedStepId | null {
    const parts = traceStepId.split(LibraryStepIds.idSeparator);
    for (let i = 1; i < parts.length - 1; i++) {
      const pin = LibraryStepIds.pinPattern.exec(parts[i]);
      if (!pin) continue;
      return {
        stepId: parts.slice(0, i).join(LibraryStepIds.idSeparator),
        ref: pin[1],
        version: Number(pin[2]),
        childStepId: parts.slice(i + 1).join(LibraryStepIds.idSeparator),
      };
    }
    return null;
  }

  /** True when the trace step came from the route step `stepId` (itself or its library expansion). */
  static belongsTo(traceStepId: string, stepId: string): boolean {
    return traceStepId === stepId || LibraryStepIds.parse(traceStepId)?.stepId === stepId;
  }

  private static escape(text: string): string {
    return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }
}
