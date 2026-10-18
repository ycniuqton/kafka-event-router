import { useCallback, useMemo, useRef, useState } from 'react';
import { Keys, useShortcut } from '../../../../../store';
import { JsCodeEditor } from '../../editor/JsCodeEditor';
import type { CodeEditorHandle, CodeMarker } from '../../editor/CodeEditorHandle';
import { useStudio } from '../../hooks/StudioContext';
import type { JsSnippet } from '../../model/JsSnippets';
import { SnippetInserter } from '../../model/SnippetInserter';
import { JsPanelTab } from '../../model/StudioEnums';
import { StepChangeLabel, StudioText } from '../../model/StudioText';
import { JsConsole } from './JsConsole';
import { JsErrorNotice } from './JsErrorNotice';
import { JsStepActions } from './JsStepActions';
import { JsStepToolbar } from './JsStepToolbar';
import { ExplainPanel } from '../../magic/ExplainPanel';
import type { StepSettingsProps } from './StepSettingsProps';
import styles from './JsStepSettings.module.css';

/** JavaScript step: Monaco editor or Console (in the card or full screen), snippets, inline error with jump, logs and explanation. */
export function JsStepSettings({ step, index, update }: StepSettingsProps) {
  const { model } = useStudio();
  const [tab, setTab] = useState<JsPanelTab>(JsPanelTab.Code);
  const [fullScreen, setFullScreen] = useState(false);
  useShortcut({ key: Keys.Escape, allowInInputs: true, handler: () => setFullScreen(false) }, fullScreen);
  const editor = useRef<CodeEditorHandle | null>(null);
  const onReady = useCallback((handle: CodeEditorHandle | null) => {
    editor.current = handle;
  }, []);
  const failure = model.report.failureAtStep(index, model.current?.id);
  const logs = model.result?.logs ?? [];
  const markers = useMemo<CodeMarker[]>(
    () => (failure?.line ? [{ line: failure.line, column: failure.column ?? 1, message: StudioText.eventError(failure.sampleId, failure.message) }] : []),
    [failure],
  );
  const insert = (snippet: JsSnippet) => {
    if (tab === JsPanelTab.Code && editor.current) editor.current.insert(snippet.code);
    else update({ code: SnippetInserter.insert(step.code, snippet.code) }, StepChangeLabel.editedJs);
  };
  return (
    <div className={fullScreen ? styles.fullScreen : styles.inline}>
      <JsStepToolbar tab={tab} onTab={setTab} onInsert={insert} fullScreen={fullScreen} onFullScreen={setFullScreen} />
      {tab === JsPanelTab.Code ? (
        <JsCodeEditor value={step.code} markers={markers} onChange={(code) => update({ code }, StepChangeLabel.editedJs)} onReady={onReady} ariaLabel="JavaScript step code" />
      ) : (
        <JsConsole index={index} />
      )}
      {failure && <JsErrorNotice failure={failure} />}
      {tab === JsPanelTab.Code && logs.length > 0 && <div className={styles.logs}>{logs.join(StudioText.lineBreak)}</div>}
      {step.explained && <ExplainPanel steps={[step]} />}
      <JsStepActions onExplain={() => update({ explained: true })} />
    </div>
  );
}
