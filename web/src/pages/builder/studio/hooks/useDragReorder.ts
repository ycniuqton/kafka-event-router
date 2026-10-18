import { useState, type DragEvent } from 'react';

/** Drag props for one reorderable item. */
export type DragHandlers = {
  draggable: true;
  onDragStart(e: DragEvent): void;
  onDragOver(e: DragEvent): void;
  onDrop(e: DragEvent): void;
  onDragEnd(): void;
};

/** HTML5 drag-and-drop reordering of a list; calls `onMove(from, to)` on drop. */
export function useDragReorder(onMove: (from: number, to: number) => void) {
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [overIndex, setOverIndex] = useState<number | null>(null);
  const reset = () => {
    setDragIndex(null);
    setOverIndex(null);
  };
  const handlersFor = (index: number): DragHandlers => ({
    draggable: true,
    onDragStart: (e) => {
      e.dataTransfer.effectAllowed = 'move';
      setDragIndex(index);
    },
    onDragOver: (e) => {
      if (dragIndex === null) return;
      e.preventDefault();
      setOverIndex(index);
    },
    onDrop: (e) => {
      e.preventDefault();
      if (dragIndex !== null) onMove(dragIndex, index);
      reset();
    },
    onDragEnd: reset,
  });
  return { handlersFor, overIndex, dragIndex };
}
