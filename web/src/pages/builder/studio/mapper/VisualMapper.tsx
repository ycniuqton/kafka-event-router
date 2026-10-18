import { useMemo, useRef, useState, type CSSProperties } from 'react';
import { Button, ButtonSize, ButtonVariant, SegmentedControl, SegmentSize } from '../../../../components/ui';
import { FieldPath, JsonFlattener, MapEntryEditor, MappedFields, MapStart, MapStartLabel, type JsonLine, type JsonObject, type MapEntry } from '../../../../domain';
import type { StepSettingsProps } from '../components/settings/StepSettingsProps';
import { useStudio } from '../hooks/StudioContext';
import { StepChangeLabel } from '../model/StudioText';
import { ConnectorLayer } from './ConnectorLayer';
import { ConnectorKey, ConnectorRegistry, type ConnectorLink } from './ConnectorRegistry';
import { MapperDrag } from './MapperDrag';
import { MapperText } from './MapperText';
import { OutputTreeEditor } from './OutputTreeEditor';
import styles from './VisualMapper.module.css';

/** Start options; "Paste an example" opens transform by example (proposed steps replace hand mapping). */
class MapperStartOptions {
  static readonly pasteExample = 'paste_example';
  static readonly all = [
    ...Object.values(MapStart).map((value) => ({ value: value as string, label: MapStartLabel[value] })),
    { value: MapperStartOptions.pasteExample, label: MapperText.pasteExample },
  ];

  static choose(value: string, apply: (start: MapStart) => void, openExample: () => void): void {
    if (value === MapperStartOptions.pasteExample) openExample();
    else apply(value as MapStart);
  }
}

/** Visual Mapper (`map` step): drag input fields into an output tree, pick value sources, see unmapped fields. */
export function VisualMapper({ step, index, update }: StepSettingsProps) {
  const { commands } = useStudio();
  const view = useMapperView(index, step.mapEntries ?? []);
  const [expanded, setExpanded] = useState(false);
  const [hover, setHover] = useState<ConnectorLink | null>(null);
  const body = useRef<HTMLDivElement>(null);
  const registry = useMemo(() => new ConnectorRegistry(), []);
  const entries = step.mapEntries ?? [];
  const start = step.mapStart ?? MapStart.CopyInput;
  const label = StepChangeLabel.edited(step.kind);
  const commit = (next: MapEntry[]) => update({ mapEntries: next }, label);
  const dropField = (path: string, parent: string) => {
    const target = [parent, MapEntryEditor.leaf(path)].filter(Boolean).join(MapEntryEditor.separator);
    const mapped = new MapEntryEditor(entries).mapField(path, target);
    commit(start === MapStart.CopyInput ? new MapEntryEditor(mapped).removeInput(path) : mapped);
  };
  const unmapped = MappedFields.unmapped(view.input, start, entries);
  return (
    <div className={[styles.mapper, expanded && styles.expanded].filter(Boolean).join(' ')} data-testid="visual-mapper">
      <div className={styles.toolbar}>
        <span className={styles.detail}>{MapperText.start}</span>
        <SegmentedControl size={SegmentSize.Xs} options={MapperStartOptions.all} value={start as string} ariaLabel={MapperText.start}
          onChange={(v) => MapperStartOptions.choose(v, (mapStart) => update({ mapStart }, label), () => commands.setUi({ byExample: true }))} />
        <span className={styles.spacer} />
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setExpanded(!expanded)}>{expanded ? MapperText.collapse : MapperText.expand}</Button>
      </div>
      <div className={styles.columns} ref={body}>
        <InputFieldList lines={view.lines} unmapped={unmapped} registry={registry} copyInput={start === MapStart.CopyInput}
          onMap={(path) => dropField(path, '')} onRemove={(path) => commit(new MapEntryEditor(entries).removeInput(path))} onHover={setHover} />
        <div className={styles.outputColumn}>
          <div className={styles.columnTitle}>{MapperText.output}</div>
          <OutputTreeEditor entries={entries} onChange={commit} onDropField={dropField} fields={view.fields} liveOf={view.liveOf} registry={registry} onHover={setHover} />
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => commit(new MapEntryEditor(entries).addConstant(MapperText.newField))}>{MapperText.addField}</Button>
        </div>
        <ConnectorLayer registry={registry} container={body} link={hover} />
      </div>
      {unmapped.length > 0 && <div className={styles.unmappedNote}>{MapperText.unmapped(unmapped.length)}</div>}
    </div>
  );
}

/** The event entering the step (from the preview), its field lines and the step's live output values. */
function useMapperView(index: number, entries: MapEntry[]) {
  const { model } = useStudio();
  const snapshots = model.preview.run?.snapshots(model.current?.id, index);
  const before = snapshots?.before?.value;
  const fallback = model.current?.value;
  const after = snapshots?.after?.value;
  return useMemo(() => {
    const input = (before ?? fallback ?? {}) as JsonObject;
    const lines = JsonFlattener.flatten(input);
    const fields = lines.filter((l) => !l.isBranch).map((l) => l.path);
    const liveOf = (path: string): unknown => (after && entries.length ? FieldPath.get(after, path) : undefined);
    return { input, lines, fields, liveOf };
  }, [before, fallback, after, entries.length]);
}

type InputFieldListProps = {
  lines: JsonLine[];
  unmapped: string[];
  registry: ConnectorRegistry;
  copyInput: boolean;
  onMap(path: string): void;
  onRemove(path: string): void;
  onHover(link: ConnectorLink | null): void;
};

/** Input fields of the current event: drag (or →) to map, ✕ to drop from a copied output; unmapped in grey. */
function InputFieldList({ lines, unmapped, registry, copyInput, onMap, onRemove, onHover }: InputFieldListProps) {
  const grey = new Set(unmapped);
  return (
    <div className={styles.inputColumn} data-testid="mapper-input">
      <div className={styles.columnTitle}>{MapperText.input}</div>
      {!lines.length && <div className={styles.dropHint}>{MapperText.noSample}</div>}
      {lines.map((line) => (
        <div key={line.path} ref={registry.ref(ConnectorKey.input(line.path))} className={styles.inputRow} data-path={line.path} data-unmapped={grey.has(line.path)}
          style={{ '--depth': line.depth } as CSSProperties} draggable onDragStart={(e) => MapperDrag.start(e, line.path)}
          onMouseEnter={() => onHover(null)}>
          <span className={styles.mono}>{line.key}</span>
          {!line.isBranch && <span className={styles.detail}>{JsonFlattener.isBranch(line.value) ? '' : String(line.value)}</span>}
          <span className={styles.spacer} />
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title={MapperText.map} aria-label={MapperText.map} onClick={() => onMap(line.path)}>→</Button>
          {copyInput && <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title={MapperText.removeInput} aria-label={MapperText.removeInput} onClick={() => onRemove(line.path)}>✕</Button>}
        </div>
      ))}
    </div>
  );
}
