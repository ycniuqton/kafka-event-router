import { useCallback, useMemo, useRef, useState } from 'react';
import { ControlSize, TextInput } from '../../../../components/ui';
import type { TransformStepKind } from '../../../../domain';
import { useDismiss } from '../hooks/useDismiss';
import { useStudio } from '../hooks/StudioContext';
import { StepListEditor } from '../model/StepListEditor';
import { StepMenuModel } from '../model/StepMenuModel';
import { StepChangeLabel } from '../model/StudioText';
import styles from './AddStepMenu.module.css';

/** "+ Add step" button opening the grouped, searchable recipe menu. */
export function AddStepMenu() {
  const { model, commands } = useStudio();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => {
    setOpen(false);
    setQuery('');
  }, []);
  useDismiss(ref, open, close);
  const groups = useMemo(() => StepMenuModel.search(query), [query]);
  const add = (kind: TransformStepKind) => {
    const step = StepMenuModel.create(kind, model.inputLines);
    commands.commit(new StepListEditor(model.steps).append([step], StepChangeLabel.added(kind)));
    commands.selectStep(step.id);
    close();
  };
  return (
    <div className={styles.wrap} ref={ref}>
      <button type="button" className={styles.add} aria-expanded={open} onClick={() => (open ? close() : setOpen(true))}>
        + Add step
      </button>
      {open && (
        <div className={styles.menu}>
          <TextInput controlSize={ControlSize.Sm} sunken block autoFocus placeholder="Search steps…" value={query} onChange={(e) => setQuery(e.target.value)} aria-label="Search steps" />
          <div className={styles.groups}>
            {groups.map((g) => (
              <div key={g.group} className={styles.group}>
                <div className={styles.groupTitle}>{g.group}</div>
                {g.items.map((item) => (
                  <button key={item.kind} type="button" className={styles.item} onClick={() => add(item.kind)}>
                    {item.label}
                  </button>
                ))}
              </div>
            ))}
            {!groups.length && <div className={styles.none}>No step matches “{query}”.</div>}
          </div>
        </div>
      )}
    </div>
  );
}
