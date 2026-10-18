import { useCallback, useRef, useState } from 'react';
import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import { useDismiss } from '../hooks/useDismiss';
import styles from './StepCardMenu.module.css';

type StepCardMenuProps = { onDuplicate(): void; onDelete(): void };

/** The ⋯ menu of a step card: Duplicate · Delete. */
export function StepCardMenu({ onDuplicate, onDelete }: StepCardMenuProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(ref, open, close);
  const choose = (run: () => void) => {
    close();
    run();
  };
  return (
    <div className={styles.wrap} ref={ref} onClick={(e) => e.stopPropagation()}>
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} square title="More" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)}>
        ⋯
      </Button>
      {open && (
        <div className={styles.menu} role="menu">
          <button type="button" role="menuitem" className={styles.item} onClick={() => choose(onDuplicate)}>
            Duplicate
          </button>
          <button type="button" role="menuitem" className={`${styles.item} ${styles.danger}`} onClick={() => choose(onDelete)}>
            Delete
          </button>
        </div>
      )}
    </div>
  );
}
