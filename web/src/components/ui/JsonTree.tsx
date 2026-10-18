import { useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import { JsonFlattener, type JsonLine } from '../../domain';
import { LineHighlight } from './variants';
import styles from './JsonTree.module.css';

export type LineDecoration = { highlight?: LineHighlight; mark?: string; title?: string; trailing?: ReactNode };

export type JsonTreeProps = {
  value: unknown;
  /** Allow collapsing objects/arrays with a ▸/▾ toggle (default true). */
  collapsible?: boolean;
  /** Paths collapsed initially. */
  initiallyCollapsed?: string[];
  onLineClick?: (line: JsonLine) => void;
  decorate?: (line: JsonLine) => LineDecoration | undefined;
  className?: string;
};

/** Value colour class by JSON type: strings --cstr, numbers --cnum, other --warn. */
class JsonValueStyle {
  static className(value: unknown): string {
    if (typeof value === 'string') return styles.string;
    return typeof value === 'number' ? styles.number : styles.other;
  }
}

/** Flattened, indented JSON view with type-coloured values (samples, studio, dead letters). */
export function JsonTree({ value, collapsible = true, initiallyCollapsed = [], onLineClick, decorate, className }: JsonTreeProps) {
  const [collapsed, setCollapsed] = useState(() => new Set(initiallyCollapsed));
  const lines = useMemo(() => JsonFlattener.flatten(value), [value]);
  const visible = lines.filter((l) => ![...collapsed].some((p) => l.path.startsWith(`${p}.`)));
  const toggle = (path: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  return (
    <div className={[styles.tree, className].filter(Boolean).join(' ')}>
      {visible.map((line) => (
        <JsonTreeLine
          key={line.path}
          line={line}
          collapsed={collapsed.has(line.path)}
          onToggle={collapsible && line.isBranch ? () => toggle(line.path) : undefined}
          onClick={onLineClick}
          decoration={decorate?.(line)}
        />
      ))}
    </div>
  );
}

type JsonTreeLineProps = {
  line: JsonLine;
  collapsed: boolean;
  onToggle?: () => void;
  onClick?: (line: JsonLine) => void;
  decoration?: LineDecoration;
};

function JsonTreeLine({ line, collapsed, onToggle, onClick, decoration }: JsonTreeLineProps) {
  const highlight = decoration?.highlight && decoration.highlight !== LineHighlight.None ? styles[decoration.highlight] : '';
  const classes = [styles.line, onClick && styles.clickable, highlight].filter(Boolean).join(' ');
  return (
    <div className={classes} style={{ '--depth': line.depth } as CSSProperties} title={decoration?.title} onClick={onClick && (() => onClick(line))}>
      {decoration?.mark !== undefined && <span className={styles.mark}>{decoration.mark}</span>}
      {onToggle && (
        <button type="button" className={styles.toggle} onClick={(e) => {
            e.stopPropagation();
            onToggle();
          }} aria-label={collapsed ? 'Expand' : 'Collapse'}>
          {collapsed ? '▸' : '▾'}
        </button>
      )}
      <span className={styles.key}>{line.key}:</span>
      {line.isBranch ? (
        <span className={styles.summary}>{line.summary}</span>
      ) : (
        <span className={`${styles.value} ${JsonValueStyle.className(line.value)}`}>{JSON.stringify(line.value)}</span>
      )}
      {decoration?.trailing && <span className={styles.trailing}>{decoration.trailing}</span>}
    </div>
  );
}
