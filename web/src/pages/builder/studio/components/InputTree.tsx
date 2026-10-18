import { useState, type CSSProperties } from 'react';
import type { JsonLine } from '../../../../domain';
import { useStudio } from '../hooks/StudioContext';
import { JsonLineKind } from '../model/JsonKind';
import { CollapsedPaths } from '../model/CollapsedPaths';
import { FieldActionBar } from './FieldActionBar';
import { JsonValueText } from './JsonValueText';
import { TypeIcon } from './TypeIcon';
import styles from './InputTree.module.css';

/** Collapsible JSON tree of the current sample; clicking a line opens the click-to-transform bar. */
export function InputTree() {
  const { model, ui, commands } = useStudio();
  const [collapsed, setCollapsed] = useState<CollapsedPaths>(() => new CollapsedPaths());
  const visible = model.inputLines.filter((l) => !collapsed.hides(l.path));
  return (
    <div className={styles.tree}>
      {visible.map((line) => (
        <InputTreeLine
          key={line.path}
          line={line}
          selected={ui.selectedLine === line.path}
          collapsed={collapsed.has(line.path)}
          onToggle={() => setCollapsed(collapsed.toggle(line.path))}
          onSelect={() => commands.selectLine(ui.selectedLine === line.path ? null : line.path)}
        />
      ))}
    </div>
  );
}

type InputTreeLineProps = { line: JsonLine; selected: boolean; collapsed: boolean; onToggle(): void; onSelect(): void };

function InputTreeLine({ line, selected, collapsed, onToggle, onSelect }: InputTreeLineProps) {
  return (
    <div className={[styles.line, selected && styles.selected].filter(Boolean).join(' ')} style={{ '--depth': line.depth } as CSSProperties} onClick={onSelect} onContextMenu={(e) => {
        e.preventDefault();
        onSelect();
      }}>
      <div className={styles.row}>
        {line.isBranch ? (
          <button type="button" className={styles.toggle} aria-label={collapsed ? 'Expand' : 'Collapse'} onClick={(e) => {
              e.stopPropagation();
              onToggle();
            }}>
            {collapsed ? '▸' : '▾'}
          </button>
        ) : (
          <span className={styles.toggle} />
        )}
        <TypeIcon kind={JsonLineKind.of(line)} />
        <span className={styles.key}>{line.key}:</span>
        <JsonValueText line={line} />
      </div>
      {selected && <FieldActionBar line={line} />}
    </div>
  );
}
