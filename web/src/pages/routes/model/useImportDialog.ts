import { useState, type ChangeEvent } from 'react';
import { ImportMode, type ImportReport } from '../../../domain';
import { actions } from '../../../store';

/** State of the Import dialog: text (file or paste), mode, the last report, busy and error. */
export function useImportDialog(onClose: () => void) {
  const [content, setContentState] = useState('');
  const [fileName, setFileName] = useState('');
  const [mode, setModeState] = useState<ImportMode>(ImportMode.Create);
  const [report, setReport] = useState<ImportReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const run = async (dryRun: boolean) => {
    setBusy(true);
    setError('');
    try {
      setReport(await actions.routes.importRoutes(content, { mode, dryRun }));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const setContent = (text: string) => {
    setContentState(text);
    setReport(null);
  };
  const pickFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setFileName(file.name);
    setContent(await file.text());
  };
  const setMode = (next: ImportMode) => {
    setModeState(next);
    setReport(null);
  };
  const close = () => {
    setContent('');
    setFileName('');
    onClose();
  };
  return { content, fileName, mode, report, busy, error, run, setContent, pickFile, setMode, close };
}
