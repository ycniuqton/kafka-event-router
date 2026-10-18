import type { RouteExport } from '../../../domain';

/** Saves an exported route file in the browser. */
export class RouteFileSaver {
  static save(file: RouteExport): void {
    const url = URL.createObjectURL(new Blob([file.content], { type: file.mediaType }));
    const link = document.createElement('a');
    link.href = url;
    link.download = file.fileName;
    link.click();
    URL.revokeObjectURL(url);
  }
}
