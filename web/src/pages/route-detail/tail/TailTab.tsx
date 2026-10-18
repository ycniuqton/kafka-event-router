import { useMemo, useState } from 'react';
import { Card, ToneBadge } from '../../../components/ui';
import { Phase, TailShow, TailState, TailStateMeta, Tone, type Route, type TailFilter } from '../../../domain';
import { PhaseTag } from '../components/PhaseTag';
import { TailControls } from './TailControls';
import { TailRow } from './TailRow';
import { useLiveTail } from './useLiveTail';
import styles from './TailTab.module.css';

/** Live tail (spec §10.2): input vs output per event over `WS /routes/{id}/tail`, ≤ 20 events/s, 500 rows. */
export function TailTab({ route }: { route: Route }) {
  return <TailStream key={`${route.id}:${route.status}`} route={route} />;
}

function TailStream({ route }: { route: Route }) {
  const [paused, setPaused] = useState(false);
  const [text, setText] = useState('');
  const [show, setShow] = useState<TailShow>(TailShow.All);
  const [openSeq, setOpenSeq] = useState<number | null>(null);
  const filter = useMemo<TailFilter>(() => ({ show, text }), [show, text]);
  const { rows, state, masked, total } = useLiveTail(route.id, paused, filter);
  const meta = TailStateMeta[state];
  const usable = state !== TailState.NotRunning && state !== TailState.Unavailable;
  const setFilter = (f: TailFilter) => {
    setText(f.text);
    setShow(f.show);
  };
  return (
    <Card>
      <div className={styles.head}>
        <span className={styles.pulse} data-live={meta.live} data-tone={meta.tone} />
        <span className={styles.title} data-testid="tail-state">
          {meta.label}
        </span>
        <PhaseTag phase={Phase.P3} />
        {masked && <ToneBadge tone={Tone.Warn}>Masked</ToneBadge>}
        <span className={styles.note}>{total} received · max 20 events/s</span>
      </div>
      <TailControls paused={paused} disabled={!usable} filter={filter} onPause={setPaused} onFilter={setFilter} />
      <div className={styles.scroll}>
        <div className={`${styles.grid} ${styles.header}`}>
          <span>Time</span>
          <span>Offset</span>
          <span />
          <span>Input</span>
          <span>Output</span>
        </div>
        {rows.map((row) => (
          <TailRow key={row.seq} row={row} open={openSeq === row.seq} onToggle={() => setOpenSeq(openSeq === row.seq ? null : row.seq)} />
        ))}
        {!rows.length && <div className={styles.empty}>{TailEmptyText.of(state)}</div>}
      </div>
    </Card>
  );
}

/** Empty-list copy per state. */
class TailEmptyText {
  static of(state: TailState): string {
    if (state === TailState.NotRunning) return 'Start or resume the route to watch events pass.';
    if (state === TailState.Unavailable) return 'The live tail is unavailable right now. Retrying…';
    return 'No events match. Clear the filter or wait for new events.';
  }
}
