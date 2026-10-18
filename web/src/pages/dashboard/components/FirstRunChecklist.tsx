import { useNavigate } from 'react-router-dom';
import { Card, CardHeader } from '../../../components/ui';
import { FirstRunChecklistManager, type FirstRunState } from '../model/FirstRun';
import { DemoDataCard } from './DemoDataCard';
import { FirstRunStepRow } from './FirstRunStepRow';
import styles from './FirstRunChecklist.module.css';

/** New-workspace checklist: connection → route → deploy, plus the demo-data shortcut (spec §12). */
export function FirstRunChecklist({ state }: { state: FirstRunState }) {
  const navigate = useNavigate();
  const current = FirstRunChecklistManager.currentIndex(state);
  return (
    <Card>
      <CardHeader title="Get started" meta="Three steps to your first flowing route" />
      <ol className={styles.steps}>
        {FirstRunChecklistManager.steps.map((step, i) => (
          <FirstRunStepRow key={step.id} step={step} index={i} done={step.isDone(state)} current={i === current} onAct={() => step.perform(state, navigate)} />
        ))}
      </ol>
      <DemoDataCard />
    </Card>
  );
}
