import { useState, type MouseEvent } from 'react';
import { BranchSet, type RouteDraft } from '../../../domain';
import type { DraftEditor } from '../model/DraftSession';
import styles from './BranchRail.module.css';

type BranchRailProps = { draft: RouteDraft; editor: DraftEditor };

/** Branch switcher of the rail (P2): one chip per parallel path, rename the edited one, remove any but the last. */
export function BranchRail({ draft, editor }: BranchRailProps) {
  const branches = BranchSet.all(draft);
  const state = BranchSet.state(draft);
  if (branches.length < 2) return null;
  const remove = (index: number) => (e: MouseEvent) => {
    e.stopPropagation();
    editor.removeBranch(index);
  };
  return (
    <div className={styles.branches}>
      <span className={styles.title}>Branches · editing</span>
      <div className={styles.chips}>
        {branches.map((b, i) => (
          <span
            key={b.id}
            role="button"
            tabIndex={0}
            className={styles.chip}
            data-active={i === state.index || undefined}
            onClick={() => editor.selectBranch(i)}
            onKeyDown={(e) => e.key === 'Enter' && editor.selectBranch(i)}
          >
            {b.id}
            <button type="button" className={styles.remove} aria-label={`Remove branch ${b.id}`} title="Remove branch" onClick={remove(i)}>
              ×
            </button>
          </span>
        ))}
      </div>
      <BranchName key={state.id} id={state.id} onRename={(id) => editor.renameBranch(id)} />
    </div>
  );
}

/** Inline rename of the edited branch; applied on blur / Enter when the id is valid. */
function BranchName({ id, onRename }: { id: string; onRename: (id: string) => void }) {
  const [value, setValue] = useState(id);
  const valid = BranchSet.isValidId(value);
  const apply = () => {
    if (valid && value !== id) onRename(value);
    else setValue(id);
  };
  return (
    <input
      className={styles.rename}
      aria-label="Branch name"
      value={value}
      data-invalid={!valid || undefined}
      onChange={(e) => setValue(e.target.value)}
      onBlur={apply}
      onKeyDown={(e) => e.key === 'Enter' && apply()}
    />
  );
}
