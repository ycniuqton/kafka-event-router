import { useRef, useState, type KeyboardEvent } from 'react';
import { ControlSize, TextInput } from '../../../../components/ui';
import { StudioText } from '../model/StudioText';
import { FormulaCompletion, type FormulaCompletionResult } from './FormulaCompletion';
import styles from './VisualMapper.module.css';

/** Keys that accept or dismiss a suggestion. */
const CompletionKey = { Accept: ['Enter', 'Tab'], Close: 'Escape', Down: 'ArrowDown', Up: 'ArrowUp' } as const;

type FormulaBarProps = {
  value: string;
  onChange(value: string): void;
  /** Input field paths (relative to value) offered by autocomplete. */
  fields: string[];
  /** Result of the formula on the current event from the last preview run (undefined: not computed). */
  live?: unknown;
};

/** Formula input with autocomplete of fields and engine functions, plus the live result on the current event. */
export function FormulaBar({ value, onChange, fields, live }: FormulaBarProps) {
  const input = useRef<HTMLInputElement>(null);
  const [completion, setCompletion] = useState<FormulaCompletionResult | null>(null);
  const [active, setActive] = useState(0);
  const refresh = (text: string, caret: number) => {
    const next = FormulaCompletion.suggest(text, caret, fields);
    setCompletion(next.items.length ? next : null);
    setActive(0);
  };
  const accept = (insert: string) => {
    if (!completion) return;
    const next = FormulaCompletion.apply(value, completion, insert);
    onChange(next.text);
    setCompletion(null);
    requestAnimationFrame(() => input.current?.setSelectionRange(next.caret, next.caret));
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (!completion) return;
    if ((CompletionKey.Accept as readonly string[]).includes(e.key)) {
      e.preventDefault();
      accept(completion.items[active].insert);
    } else if (e.key === CompletionKey.Close) setCompletion(null);
    else if (e.key === CompletionKey.Down) setActive((a) => Math.min(a + 1, completion.items.length - 1));
    else if (e.key === CompletionKey.Up) setActive((a) => Math.max(a - 1, 0));
  };
  return (
    <div className={styles.formula}>
      <span className={styles.formulaSign}>=</span>
      <TextInput
        onFocus={(e) => (input.current = e.currentTarget)}
        controlSize={ControlSize.Sm}
        mono
        sunken
        className={styles.grow}
        placeholder={StudioText.formulaPlaceholder}
        value={value}
        aria-label="Formula"
        aria-autocomplete="list"
        onChange={(e) => {
          onChange(e.target.value);
          refresh(e.target.value, e.target.selectionStart ?? e.target.value.length);
        }}
        onKeyDown={onKeyDown}
        onBlur={() => setCompletion(null)}
      />
      {completion && (
        <ul className={styles.suggestions} role="listbox">
          {completion.items.map((item, i) => (
            <li key={item.label} role="option" aria-selected={i === active} className={styles.suggestion} data-active={i === active} onMouseDown={(e) => { e.preventDefault(); accept(item.insert); }}>
              <span className={styles.mono}>{item.label}</span>
              <span className={styles.detail}>{item.detail}</span>
            </li>
          ))}
        </ul>
      )}
      {live !== undefined && <span className={styles.live} title="Result on the current event (from the preview)">→ {StudioText.exampleValue(live)}</span>}
    </div>
  );
}
