import { useState, type CSSProperties, type DragEvent } from 'react';
import { Button, ButtonSize, ButtonVariant, ControlSize, TextInput } from '../../../../components/ui';
import { MapEntryEditor, ValueSourceKind, type MapEntry } from '../../../../domain';
import { ConnectorKey, type ConnectorLink, type ConnectorRegistry } from './ConnectorRegistry';
import { MapperDrag } from './MapperDrag';
import { OutputRowType, OutputTreeModel, type OutputRow } from './OutputTreeModel';
import { ValueSourcePicker } from './ValueSourcePicker';
import { MapperText } from './MapperText';
import styles from './VisualMapper.module.css';

type OutputTreeEditorProps = {
  entries: MapEntry[];
  onChange(entries: MapEntry[]): void;
  /** Maps a dropped input field to `parent.<name>` (parent '' = root). */
  onDropField(path: string, parent: string): void;
  fields: string[];
  liveOf(path: string): unknown;
  registry: ConnectorRegistry;
  onHover(link: ConnectorLink | null): void;
};

/** Output tree of the Visual Mapper: rename, re-source, nest / un-nest and remove output fields; drop input fields. */
export function OutputTreeEditor(props: OutputTreeEditorProps) {
  const rows = OutputTreeModel.rows(props.entries);
  return (
    <div className={styles.outputTree} onDragOver={MapperDrag.over} onDrop={(e) => dropOn(e, '', props)} data-testid="mapper-output">
      {!rows.length && <div className={styles.dropHint}>{MapperText.dropHint}</div>}
      {rows.map((row) => (row.type === OutputRowType.Group ? <GroupRow key={`g:${row.path}`} row={row} {...props} /> : <EntryRow key={`e:${row.index}`} row={row} {...props} />))}
    </div>
  );
}

function dropOn(e: DragEvent, parent: string, props: OutputTreeEditorProps) {
  const path = MapperDrag.drop(e);
  if (path) props.onDropField(path, parent);
}

/** An object of the output tree (drop a field on it to nest the field inside). */
function GroupRow({ row, ...props }: { row: Extract<OutputRow, { type: typeof OutputRowType.Group }> } & OutputTreeEditorProps) {
  return (
    <div className={styles.groupRow} style={{ '--depth': row.depth } as CSSProperties} onDragOver={MapperDrag.over} onDrop={(e) => dropOn(e, row.path, props)}>
      <span className={styles.mono}>{row.name}</span>
      <span className={styles.detail}>{'{ }'}</span>
    </div>
  );
}

/** One output field: name, source (or removal) and its actions. */
function EntryRow({ row, ...props }: { row: Extract<OutputRow, { type: typeof OutputRowType.Entry }> } & OutputTreeEditorProps) {
  const { entry, index } = row;
  const editor = new MapEntryEditor(props.entries);
  const link = entry.sourceKind === ValueSourceKind.Field && !entry.remove ? { from: ConnectorKey.input(entry.argument), to: ConnectorKey.output(index) } : null;
  return (
    <div
      ref={props.registry.ref(ConnectorKey.output(index))}
      className={styles.entryRow}
      data-removed={!!entry.remove}
      style={{ '--depth': row.depth } as CSSProperties}
      onMouseEnter={() => props.onHover(link)}
      onMouseLeave={() => props.onHover(null)}
    >
      <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.name} value={row.name} aria-label={MapperText.fieldName} onChange={(e) => props.onChange(editor.rename(index, e.target.value))} />
      {entry.remove ? (
        <span className={styles.removed}>{MapperText.removed}</span>
      ) : (
        <ValueSourcePicker sourceKind={entry.sourceKind} argument={entry.argument} fields={props.fields} live={props.liveOf(entry.path)} onChange={(patch) => props.onChange(editor.update(index, patch))} />
      )}
      <EntryActions index={index} depth={row.depth} removal={!!entry.remove} {...props} />
    </div>
  );
}

/** Nest into an object, un-nest, delete (or restore a removed field). */
function EntryActions({ index, depth, removal, entries, onChange }: { index: number; depth: number; removal: boolean } & OutputTreeEditorProps) {
  const [group, setGroup] = useState<string | null>(null);
  const editor = new MapEntryEditor(entries);
  if (group !== null) {
    const apply = () => {
      if (group.trim()) onChange(editor.nest(index, group.trim()));
      setGroup(null);
    };
    return (
      <TextInput autoFocus controlSize={ControlSize.Sm} mono sunken className={styles.groupInput} placeholder={MapperText.groupPlaceholder} value={group} aria-label={MapperText.groupName}
        onChange={(e) => setGroup(e.target.value)} onBlur={apply} onKeyDown={(e) => e.key === MapperText.enterKey && apply()} />
    );
  }
  return (
    <div className={styles.entryActions}>
      {!removal && <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title={MapperText.nest} aria-label={MapperText.nest} onClick={() => setGroup('')}>↳</Button>}
      {!removal && <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title={MapperText.unnest} aria-label={MapperText.unnest} disabled={depth === 0} onClick={() => onChange(editor.unnest(index))}>↰</Button>}
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title={removal ? MapperText.restore : MapperText.delete} aria-label={removal ? MapperText.restore : MapperText.delete} onClick={() => onChange(editor.delete(index))}>{removal ? '↺' : '✕'}</Button>
    </div>
  );
}
