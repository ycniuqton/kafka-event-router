import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import { ProposalSelection, ProposalText } from './ProposalSelection';
import styles from './Magic.module.css';

type ProposalDiffProps = { selection: ProposalSelection; onChange(next: ProposalSelection): void; onAccept(): void; onReject(): void };

/** Proposed steps as a diff: each line can be unticked, low confidence is amber, blocked steps cannot be accepted. */
export function ProposalDiff({ selection, onChange, onAccept, onReject }: ProposalDiffProps) {
  return (
    <div className={styles.diff} aria-label="Proposed steps">
      {selection.proposals.map((p, i) => (
        <div key={p.step.id}>
          <label className={styles.proposal} data-low={ProposalSelection.isLow(p)} data-blocked={!!p.issue} title={ProposalSelection.isLow(p) ? ProposalText.lowConfidence : undefined}>
            <input type="checkbox" checked={selection.isSelected(i)} disabled={!selection.canSelect(i)} onChange={() => onChange(selection.toggle(i))} />
            <span className={styles.proposalText}>+ {p.text}</span>
            {p.confidence !== undefined && <span className={styles.confidence}>{ProposalText.confidence(p.confidence)}</span>}
          </label>
          {p.issue && <div className={styles.issue}>{ProposalText.blocked + p.issue}</div>}
        </div>
      ))}
      <div className={styles.decide}>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Primary} disabled={!selection.count} onClick={onAccept}>
          {selection.count === selection.proposals.length ? ProposalText.accept : ProposalText.acceptSelected(selection.count)}
        </Button>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={onReject}>
          {ProposalText.reject}
        </Button>
      </div>
    </div>
  );
}
