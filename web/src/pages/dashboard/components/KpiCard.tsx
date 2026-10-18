import type { ReactNode } from 'react';
import { Card } from '../../../components/ui';
import type { Tone } from '../../../domain';
import { Keys } from '../../../store';
import styles from './KpiCard.module.css';

export type KpiCardProps = {
  label: ReactNode;
  value: ReactNode;
  valueTone?: Tone;
  footer?: ReactNode;
  aside?: ReactNode;
  onClick?: () => void;
};

/** Health card shell: muted label, 26px figure, optional right-hand visual and footer line. */
export function KpiCard({ label, value, valueTone, footer, aside, onClick }: KpiCardProps) {
  return (
    <Card
      padding="md"
      interactive={!!onClick}
      onClick={onClick}
      role={onClick ? 'link' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={(e) => e.key === Keys.Enter && onClick?.()}
    >
      <div className={styles.label}>{label}</div>
      <div className={styles.body}>
        <div className={styles.value} data-tone={valueTone}>
          {value}
        </div>
        {aside}
      </div>
      {footer && <div className={styles.footer}>{footer}</div>}
    </Card>
  );
}
