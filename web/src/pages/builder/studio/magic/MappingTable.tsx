import type { InferredMapping } from '../../../../api';
import { ProposalSelection, ProposalText } from './ProposalSelection';
import styles from './Magic.module.css';

/** Texts of the mapping table. */
class MappingText {
  static readonly target = 'Output field';
  static readonly source = 'From';
  static readonly reason = 'Matched by';
  static readonly confidence = 'Conf.';
  static readonly constant = 'fixed value';
  static readonly unmapped = 'Input fields not in the example (removed): ';
}

/** Where each output field of the example comes from, low confidence in amber. */
export function MappingTable({ inferred }: { inferred: InferredMapping }) {
  return (
    <>
      <div className={styles.mappings} aria-label="Field mappings">
        <span className={styles.mappingHead}>{MappingText.target}</span>
        <span className={styles.mappingHead}>{MappingText.source}</span>
        <span className={styles.mappingHead}>{MappingText.reason}</span>
        <span className={styles.mappingHead}>{MappingText.confidence}</span>
        {inferred.mappings.map((m) => {
          const low = m.confidence < ProposalSelection.lowConfidence ? styles.mappingLow : undefined;
          return [
            <span key={`${m.target}-t`} className={low}>{m.target}</span>,
            <span key={`${m.target}-s`} className={low}>{m.source ?? MappingText.constant}</span>,
            <span key={`${m.target}-r`} className={low}>{m.reason}</span>,
            <span key={`${m.target}-c`} className={low}>{ProposalText.confidence(m.confidence)}</span>,
          ];
        })}
      </div>
      {inferred.unmapped.length > 0 && <div className={styles.unmapped}>{MappingText.unmapped + inferred.unmapped.join(', ')}</div>}
    </>
  );
}
