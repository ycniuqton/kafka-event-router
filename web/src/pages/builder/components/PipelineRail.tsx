import { useSettings } from '../../../store';
import { BuilderStep, type RouteDraft } from '../../../domain';
import type { DraftEditor } from '../model/DraftSession';
import type { RailCardView } from '../model/RailModel';
import { BranchRail } from './BranchRail';
import { RailCard } from './RailCard';
import styles from './PipelineRail.module.css';

export type PipelineRailProps = {
  cards: RailCardView[];
  active: BuilderStep;
  onSelect: (step: BuilderStep) => void;
  draft: RouteDraft;
  editor: DraftEditor;
};

/** Left rail: Source, the branch switcher (P2), the edited branch's Filter → "+ Add branch" → Transform → Destination, Review & deploy. */
export function PipelineRail({ cards, active, onSelect, draft, editor }: PipelineRailProps) {
  const { showPhases } = useSettings();
  return (
    <aside className={styles.rail}>
      <div className={styles.label}>Pipeline</div>
      {cards.map((card) => (
        <div key={card.step} className={styles.link}>
          <RailCard card={card} active={card.step === active} onClick={() => onSelect(card.step)} />
          <div className={styles.connector} />
          {card.step === BuilderStep.Source && <BranchRail draft={draft} editor={editor} />}
          {card.step === BuilderStep.Filter && (
            <button type="button" className={styles.branch} onClick={() => editor.addBranch()}>
              + Add branch {showPhases && '· P2'}
            </button>
          )}
        </div>
      ))}
      <button type="button" className={styles.review} data-active={active === BuilderStep.Review || undefined} onClick={() => onSelect(BuilderStep.Review)}>
        Review &amp; deploy
      </button>
    </aside>
  );
}
