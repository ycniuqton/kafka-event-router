import { useEffect, useMemo, useRef, useState } from 'react';
import { services } from '../../../api';
import { TailState, type TailFilter } from '../../../domain';
import { TailBuffer, type TailRow } from './TailBuffer';
import { TailSocketClient } from './TailSocketClient';

export type LiveTailView = { rows: TailRow[]; state: TailState; masked: boolean; total: number };

/** Opens the route's tail socket while mounted; pause and filter are forwarded to the server. */
export function useLiveTail(routeId: string, paused: boolean, filter: TailFilter): LiveTailView {
  const [rows, setRows] = useState<TailRow[]>([]);
  const [state, setState] = useState<TailState>(TailState.Connecting);
  const [masked, setMasked] = useState(false);
  const [total, setTotal] = useState(0);
  const client = useRef<TailSocketClient | null>(null);
  const seq = useRef(0);

  useEffect(() => {
    const c = new TailSocketClient(services.liveTail, routeId, {
      onRecord: (record, m) => {
        setMasked(m);
        setTotal((n) => n + 1);
        setRows((prev) => TailBuffer.add(prev, { seq: seq.current++, record, masked: m }));
      },
      onState: (s, m) => {
        setState(s);
        setMasked(m);
      },
    });
    client.current = c;
    c.start();
    return () => {
      c.stop();
      client.current = null;
    };
  }, [routeId]);

  useEffect(() => client.current?.setPaused(paused), [paused]);
  useEffect(() => client.current?.setFilter(filter), [filter]);

  const visible = useMemo(() => TailBuffer.visible(rows, filter), [rows, filter]);
  return { rows: visible, state, masked, total };
}
