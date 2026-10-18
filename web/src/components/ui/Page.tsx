import type { CSSProperties, ReactNode } from 'react';
import { PageWidth } from './variants';
import styles from './Page.module.css';

export type PageProps = { width?: number; relaxed?: boolean; children: ReactNode };

/** Scrollable page body with the design's padding (24/28/40) and max width. */
export function Page({ width = PageWidth.Wide, relaxed, children }: PageProps) {
  return (
    <div className={[styles.page, relaxed && styles.relaxed].filter(Boolean).join(' ')} style={{ '--page-w': `${width}px` } as CSSProperties}>
      {children}
    </div>
  );
}

export type PageHeaderProps = {
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  /** Breadcrumb-style back link rendered before the title, e.g. "Routes /". */
  back?: { label: string; onClick: () => void };
  /** Extra inline content after the title (badges, version). */
  children?: ReactNode;
};

/** H1 (20px/600) with muted subtitle and right-aligned actions. */
export function PageHeader({ title, subtitle, actions, back, children }: PageHeaderProps) {
  return (
    <div className={[styles.header, (actions || children) && styles.centered].filter(Boolean).join(' ')}>
      {back && (
        <button type="button" className={styles.back} onClick={back.onClick}>
          {back.label}
        </button>
      )}
      <h1 className={styles.title}>{title}</h1>
      {children}
      {subtitle && <span className={styles.subtitle}>{subtitle}</span>}
      {actions && (
        <>
          <span className={styles.spacer} />
          <div className={styles.actions}>{actions}</div>
        </>
      )}
    </div>
  );
}
