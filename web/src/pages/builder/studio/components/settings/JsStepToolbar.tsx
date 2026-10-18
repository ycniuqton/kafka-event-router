import { Button, ButtonSize, ButtonVariant, SegmentedControl, SegmentSize } from '../../../../../components/ui';
import type { JsSnippet } from '../../model/JsSnippets';
import { JsPanelTab } from '../../model/StudioEnums';
import { SnippetMenu } from './SnippetMenu';
import styles from './JsStepSettings.module.css';

/** Code / Console switch of a JS step. */
class JsPanelTabs {
  static readonly options = [
    { value: JsPanelTab.Code, label: 'Code' },
    { value: JsPanelTab.Console, label: 'Console' },
  ];
}

type JsStepToolbarProps = { tab: JsPanelTab; onTab(tab: JsPanelTab): void; onInsert(snippet: JsSnippet): void; fullScreen: boolean; onFullScreen(on: boolean): void };

/** Top row of a JS step: Code/Console tabs, the Snippets menu and the full-screen toggle. */
export function JsStepToolbar({ tab, onTab, onInsert, fullScreen, onFullScreen }: JsStepToolbarProps) {
  return (
    <div className={styles.toolbar}>
      <SegmentedControl size={SegmentSize.Xs} options={JsPanelTabs.options} value={tab} onChange={onTab} ariaLabel="JavaScript step view" />
      <span className={styles.spacer} />
      <SnippetMenu onInsert={onInsert} />
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} aria-pressed={fullScreen} onClick={() => onFullScreen(!fullScreen)}>
        {fullScreen ? 'Exit full screen' : '⤢ Full screen'}
      </Button>
    </div>
  );
}
