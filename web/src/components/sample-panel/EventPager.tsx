import { SamplePanelModel } from './SamplePanelModel';
import styles from './EventPager.module.css';

export type EventPagerProps = { page: number; total: number; onPage: (page: number) => void };

/** "1–50 of 186" with previous / next; hidden when everything fits on one page. */
export function EventPager({ page, total, onPage }: EventPagerProps) {
  const size = SamplePanelModel.pageSize;
  const pages = Math.max(1, Math.ceil(total / size));
  if (pages <= 1) return null;
  const from = page * size + 1;
  const to = Math.min(total, from + size - 1);
  return (
    <div className={styles.pager}>
      <button type="button" className={styles.nav} disabled={page === 0} onClick={() => onPage(page - 1)} aria-label="Previous page">
        ‹
      </button>
      <span>
        {from}–{to} of {total}
      </span>
      <button type="button" className={styles.nav} disabled={page >= pages - 1} onClick={() => onPage(page + 1)} aria-label="Next page">
        ›
      </button>
    </div>
  );
}
