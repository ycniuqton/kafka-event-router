import type { DragEvent } from 'react';

/** Drag-and-drop payload of the mapper: an input field path. */
export class MapperDrag {
  static readonly mime = 'application/x-router-map-field';

  static start(e: DragEvent, path: string): void {
    e.dataTransfer.setData(MapperDrag.mime, path);
    e.dataTransfer.effectAllowed = 'copy';
  }

  /** Accepts the drag over a drop target when it carries a field. */
  static over(e: DragEvent): void {
    if (!e.dataTransfer.types.includes(MapperDrag.mime)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
  }

  /** The dropped field path, or null. */
  static drop(e: DragEvent): string | null {
    const path = e.dataTransfer.getData(MapperDrag.mime);
    if (!path) return null;
    e.preventDefault();
    e.stopPropagation();
    return path;
  }
}
