import { useState, type FormEvent } from 'react';
import { Select, TextInput } from '../../../components/ui';
import type { DeadLetter } from '../../../domain';
import { DeadLetterFacetOptions } from '../model/DeadLetterBrowserModel';
import { AnyValueFilter, DeadLetterQuery, ResolvedShown, type DeadLetterView } from '../model/DeadLetterQuery';
import styles from './DeadLetterToolbar.module.css';

export type DeadLetterRefineBarProps = { view: DeadLetterView; items: DeadLetter[]; onParam: (key: string, value: string | null) => void };

/** P2 filters: failing step, route version, message text (applied on Enter / blur) and "Show resolved". */
export function DeadLetterRefineBar({ view, items, onParam }: DeadLetterRefineBarProps) {
  const [draft, setDraft] = useState<{ base: string; text: string }>({ base: view.text ?? '', text: view.text ?? '' });
  const text = draft.base === (view.text ?? '') ? draft.text : (view.text ?? '');
  const commit = (e?: FormEvent) => {
    e?.preventDefault();
    if (text.trim() !== (view.text ?? '')) onParam(DeadLetterQuery.Text, text.trim() || null);
  };
  return (
    <form className={styles.refine} onSubmit={commit}>
      <Select controlSize="sm" value={view.stepId ?? AnyValueFilter} aria-label="Step" onChange={(e) => onParam(DeadLetterQuery.Step, e.target.value || null)}>
        <option value={AnyValueFilter}>Any step</option>
        {DeadLetterFacetOptions.steps(items, view.stepId).map((s) => (
          <option key={s.id} value={s.id}>
            {s.label}
          </option>
        ))}
      </Select>
      <Select controlSize="sm" value={view.versionNo ? String(view.versionNo) : AnyValueFilter} aria-label="Version" onChange={(e) => onParam(DeadLetterQuery.Version, e.target.value || null)}>
        <option value={AnyValueFilter}>Any version</option>
        {DeadLetterFacetOptions.versions(items, view.versionNo).map((v) => (
          <option key={v} value={String(v)}>
            v{v}
          </option>
        ))}
      </Select>
      <TextInput
        controlSize="sm"
        className={styles.search}
        value={text}
        placeholder="Search error messages"
        aria-label="Search error messages"
        onChange={(e) => setDraft({ base: view.text ?? '', text: e.target.value })}
        onBlur={() => commit()}
      />
      <label className={styles.toggle}>
        <input
          type="checkbox"
          className={styles.check}
          checked={view.includeResolved}
          onChange={(e) => onParam(DeadLetterQuery.Resolved, e.target.checked ? ResolvedShown : null)}
        />
        Show resolved
      </label>
    </form>
  );
}
