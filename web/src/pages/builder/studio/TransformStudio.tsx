import { Tabs } from '../../../components/ui';
import type { TransformStudioProps } from './contract';
import { InputPane } from './components/InputPane';
import { OutputPane } from './components/OutputPane';
import { StepsPane } from './components/StepsPane';
import { StudioContext } from './hooks/StudioContext';
import { useStudioState } from './hooks/useStudioState';
import { SidePane } from './model/StudioEnums';
import styles from './TransformStudio.module.css';

/** Tabs shown when Input and Output share one column (below 1440px). */
class SidePaneTabs {
  static readonly items = [
    { id: SidePane.Input, label: 'Input' },
    { id: SidePane.Output, label: 'Output' },
  ];
}

/** Transform Studio (builder step=transform): Input · Transform steps · Output, live on sample events (spec §8). */
export function TransformStudio(props: TransformStudioProps) {
  const studio = useStudioState(props);
  const { ui, commands } = studio;
  return (
    <StudioContext.Provider value={studio}>
      <div className={styles.studio} data-pane={ui.sidePane}>
        <div className={styles.sideTabs}>
          <Tabs items={SidePaneTabs.items} value={ui.sidePane} onChange={(sidePane) => commands.setUi({ sidePane })} />
        </div>
        <InputPane className={styles.input} />
        <StepsPane className={styles.steps} />
        <OutputPane className={styles.output} />
      </div>
    </StudioContext.Provider>
  );
}
