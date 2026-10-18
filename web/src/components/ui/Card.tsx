import type { HTMLAttributes, ReactNode } from 'react';
import { CardOverflow, CardPadding } from './variants';
import styles from './Card.module.css';

export type CardProps = HTMLAttributes<HTMLDivElement> & {
  padding?: CardPadding;
  overflow?: CardOverflow;
  /** Pointer cursor and hover border (clickable cards). */
  interactive?: boolean;
};

/** Panel surface: --p1 background, 1px --ln border, 10px radius. Alias: Panel. */
export function Card({ padding = 'none', overflow = 'clip', interactive, className, ...rest }: CardProps) {
  const classes = [
    styles.card,
    padding !== CardPadding.None && styles[`pad-${padding}`],
    overflow !== CardOverflow.Visible && styles[overflow],
    interactive && styles.interactive,
    className,
  ]
    .filter(Boolean)
    .join(' ');
  return <div className={classes} {...rest} />;
}

export const Panel = Card;

export type CardHeaderProps = { title: ReactNode; meta?: ReactNode; actions?: ReactNode };

/** Card title row with optional secondary text and right-aligned actions. */
export function CardHeader({ title, meta, actions }: CardHeaderProps) {
  return (
    <div className={styles.header}>
      <div className={styles.title}>{title}</div>
      {meta !== undefined && <div className={styles.meta}>{meta}</div>}
      {actions && <div className={styles.actions}>{actions}</div>}
    </div>
  );
}
