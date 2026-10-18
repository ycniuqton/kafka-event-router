import { Button, SegmentedControl, TextInput } from '../../../components/ui';
import { TailShowRules, type TailFilter, type TailShow } from '../../../domain';
import styles from './TailTab.module.css';

type TailControlsProps = {
  paused: boolean;
  disabled: boolean;
  filter: TailFilter;
  onPause: (paused: boolean) => void;
  onFilter: (filter: TailFilter) => void;
};

/** Pause/resume, free-text filter and the show-only choice (only dropped / only errors). */
export function TailControls({ paused, disabled, filter, onPause, onFilter }: TailControlsProps) {
  return (
    <div className={styles.controls}>
      <Button size="sm" onClick={() => onPause(!paused)} disabled={disabled}>
        {paused ? 'Resume stream' : 'Pause stream'}
      </Button>
      <TextInput
        controlSize="sm"
        mono
        className={styles.filter}
        value={filter.text}
        onChange={(e) => onFilter({ ...filter, text: e.target.value })}
        placeholder="Filter the tail…"
        aria-label="Filter the tail"
      />
      <SegmentedControl<TailShow> options={[...TailShowRules.options]} value={filter.show} onChange={(show) => onFilter({ ...filter, show })} size="xs" ariaLabel="Show" />
    </div>
  );
}
