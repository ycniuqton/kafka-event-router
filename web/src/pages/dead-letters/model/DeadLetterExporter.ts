import { DeadLetterFileFormatLabel, type DeadLetterExportFile, type DeadLetterFileFormat } from '../../../domain';

/** Saves an export file in the browser. */
export class DeadLetterExporter {
  static toast(format: DeadLetterFileFormat): string {
    return `Exported as ${DeadLetterFileFormatLabel[format]}`;
  }

  static download(file: DeadLetterExportFile): void {
    const url = URL.createObjectURL(file.blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = file.fileName;
    link.click();
    URL.revokeObjectURL(url);
  }
}
