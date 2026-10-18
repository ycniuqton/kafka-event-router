import { useState, type ChangeEvent } from 'react';
import { Button, ButtonSize, ControlSize, SegmentedControl, SegmentSize, Select, TextInput } from '../ui';
import { Keys } from '../../store';
import type { ValueFormat } from '../../domain';
import { FormatSelect } from './FormatSelect';
import { SampleToolbarModel, ToolbarMode } from './SampleToolbarModel';
import styles from './SampleToolbar.module.css';

export type SampleToolbarProps = {
  /** False when the draft has no source connection/topic yet: only upload is possible. */
  hasSource: boolean;
  loading: boolean;
  query: string;
  onLatest: (count: number) => void;
  onRange: (from: string, to: string) => void;
  onUpload: (text: string) => void;
  onSearch: (query: string) => void;
  onError: (message: string) => void;
  /** Decode format of the loaded session; null until something is loaded. */
  format: ValueFormat | null;
  onFormat: (format: ValueFormat) => void;
};

/** Load menu (latest N / time range / upload) and text search over the loaded samples. */
export function SampleToolbar(props: SampleToolbarProps) {
  const [mode, setMode] = useState<ToolbarMode>(props.hasSource ? ToolbarMode.Latest : ToolbarMode.Upload);
  return (
    <div className={styles.toolbar}>
      <SegmentedControl options={SampleToolbarModel.modes} value={mode} onChange={setMode} size={SegmentSize.Xs} ariaLabel="Sample source" />
      {mode === ToolbarMode.Latest && <LatestControls {...props} />}
      {mode === ToolbarMode.TimeRange && <RangeControls {...props} />}
      {mode === ToolbarMode.Upload && <UploadControl {...props} />}
      <FormatSelect value={props.format ?? SampleToolbarModel.defaultFormat} disabled={!props.format || props.loading} onChange={props.onFormat} />
      <TextInput
        controlSize={ControlSize.Sm}
        block
        placeholder="Search key, value, headers…"
        defaultValue={props.query}
        aria-label="Search samples"
        onKeyDown={(e) => e.key === Keys.Enter && props.onSearch(e.currentTarget.value)}
        onChange={(e) => !e.target.value && props.onSearch('')}
      />
    </div>
  );
}

/** Count picker + Load for the latest N events. */
function LatestControls({ hasSource, loading, onLatest }: SampleToolbarProps) {
  const [count, setCount] = useState<number>(SampleToolbarModel.counts[1]);
  return (
    <div className={styles.line}>
      <Select controlSize={ControlSize.Sm} value={count} onChange={(e) => setCount(Number(e.target.value))} aria-label="Sample count">
        {SampleToolbarModel.counts.map((n) => (
          <option key={n} value={n}>
            Latest {n}
          </option>
        ))}
      </Select>
      <Button size={ButtonSize.Sm} disabled={!hasSource || loading} onClick={() => onLatest(count)}>
        Load
      </Button>
    </div>
  );
}

/** From / to pickers for a time-range load. */
function RangeControls({ hasSource, loading, onRange, onError }: SampleToolbarProps) {
  const [from, setFrom] = useState(() => SampleToolbarModel.localInput(new Date(Date.now() - SampleToolbarModel.hourMs)));
  const [to, setTo] = useState(() => SampleToolbarModel.localInput(new Date()));
  const submit = () => {
    const a = SampleToolbarModel.instant(from);
    const b = SampleToolbarModel.instant(to);
    const error = SampleToolbarModel.rangeError(a, b);
    if (error) onError(error);
    else onRange(a as string, b as string);
  };
  return (
    <div className={styles.line}>
      <TextInput controlSize={ControlSize.Sm} type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="From" />
      <TextInput controlSize={ControlSize.Sm} type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} aria-label="To" />
      <Button size={ButtonSize.Sm} disabled={!hasSource || loading} onClick={submit}>
        Load
      </Button>
    </div>
  );
}

/** JSON array / NDJSON file picker (≤ 10 MB, checked before upload). */
function UploadControl({ loading, onUpload, onError }: SampleToolbarProps) {
  const pick = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    const error = SampleToolbarModel.tooLarge(file.size);
    if (error) return onError(error);
    onUpload(await file.text());
  };
  return (
    <label className={styles.upload} data-disabled={loading || undefined}>
      Upload JSON / NDJSON file
      <input type="file" accept={SampleToolbarModel.uploadAccept} className={styles.file} disabled={loading} onChange={(e) => void pick(e)} />
    </label>
  );
}
