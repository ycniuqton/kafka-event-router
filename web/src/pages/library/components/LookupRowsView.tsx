import { useEffect, useState, type CSSProperties } from 'react';
import { services } from '../../../api';
import { Button, ButtonSize, ButtonVariant, Mono } from '../../../components/ui';
import type { LookupRow } from '../../../domain';
import { LibraryLayout } from '../model/LibraryLayout';
import styles from './LibraryList.module.css';

/** Texts of the rows view. */
class RowsText {
  static readonly more = 'Load more';
  static readonly shown = (n: number, total: number) => `${n.toLocaleString()} of ${total.toLocaleString()} rows`;
}

/** Rows of one table version, paged by cursor ("Load more"). */
export function LookupRowsView({ id, version }: { id: string; version: number }) {
  const [rows, setRows] = useState<LookupRow[]>([]);
  const [total, setTotal] = useState(0);
  const [next, setNext] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    services.library.rows(id, version).then(
      (page) => {
        if (!live) return;
        setRows(page.rows);
        setTotal(page.total);
        setNext(page.next);
      },
      (e: Error) => live && setError(e.message),
    );
    return () => {
      live = false;
    };
  }, [id, version]);
  const more = async () => {
    const page = await services.library.rows(id, version, next);
    setRows([...rows, ...page.rows]);
    setNext(page.next);
  };
  if (error) return <div className={styles.error}>{error}</div>;
  return (
    <div className={styles.detail}>
      <span className={styles.muted}>{RowsText.shown(rows.length, total)}</span>
      <div className={styles.grid} style={{ '--cols': LibraryLayout.rowColumns } as CSSProperties} aria-label="Rows">
        {rows.map((r) => [
          <Mono key={`${r.key}-k`}>{r.key}</Mono>,
          <Mono key={`${r.key}-v`} truncate>{typeof r.value === 'string' ? r.value : JSON.stringify(r.value)}</Mono>,
          <span key={`${r.key}-s`} />,
        ])}
      </div>
      {next && <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => void more()}>{RowsText.more}</Button>}
    </div>
  );
}
