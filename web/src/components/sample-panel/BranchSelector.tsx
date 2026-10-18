import { SampleSummary, type BranchSummary } from '../../domain';
import { ControlSize, Select } from '../ui';
import styles from './BranchSelector.module.css';

export type BranchSelectorProps = { branches: BranchSummary[]; value: string | null; onChange: (branch: string) => void };

/** Branch picker of multi-branch routes (FR-14): the panel shows events and statuses through the chosen branch. */
export function BranchSelector({ branches, value, onChange }: BranchSelectorProps) {
  if (branches.length < 2) return null;
  const selected = branches.find((b) => b.branch === value) ?? branches[0];
  return (
    <div className={styles.row} data-testid="branch-selector">
      <span className={styles.label}>Branch</span>
      <Select controlSize={ControlSize.Sm} value={selected.branch} aria-label="Branch" onChange={(e) => onChange(e.target.value)}>
        {branches.map((b) => (
          <option key={b.branch} value={b.branch}>
            {b.branch}
          </option>
        ))}
      </Select>
      <span className={styles.counts} data-testid="branch-summary">
        {SampleSummary.branchLabel(selected)}
      </span>
    </div>
  );
}
