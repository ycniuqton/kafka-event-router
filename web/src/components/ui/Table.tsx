import { createContext, useContext, type CSSProperties, type HTMLAttributes, type ReactNode } from 'react';
import { Card } from './Card';
import styles from './Table.module.css';

const ColumnsContext = createContext('1fr');

export type DataTableProps = {
  /** CSS grid-template-columns shared by the head and every row. */
  columns: string;
  /** Horizontal scroll kicks in below this width (px). */
  minWidth?: number;
  children: ReactNode;
};

/** Card-wrapped CSS-grid table (routes, connections, audit, dead letters). */
export function DataTable({ columns, minWidth = 0, children }: DataTableProps) {
  return (
    <ColumnsContext.Provider value={columns}>
      <Card overflow="scroll">
        <div className={styles.inner} style={{ '--table-min-w': `${minWidth}px` } as CSSProperties}>
          {children}
        </div>
      </Card>
    </ColumnsContext.Provider>
  );
}

/** Uppercase header row. */
export function TableHead({ children }: { children: ReactNode }) {
  const columns = useContext(ColumnsContext);
  return (
    <div className={`${styles.row} ${styles.head}`} style={{ '--cols': columns } as CSSProperties} role="row">
      {children}
    </div>
  );
}

export type TableRowProps = HTMLAttributes<HTMLDivElement> & { selected?: boolean };

/** Body row; becomes hoverable/clickable when `onClick` is set. */
export function TableRow({ selected, onClick, className, style, ...rest }: TableRowProps) {
  const columns = useContext(ColumnsContext);
  const classes = [styles.row, onClick && styles.clickable, selected && styles.selected, className].filter(Boolean).join(' ');
  return <div role="row" className={classes} onClick={onClick} style={{ '--cols': columns, ...style } as CSSProperties} {...rest} />;
}

/** Single-line ellipsised cell. */
export function TableCell({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div className={[styles.cell, className].filter(Boolean).join(' ')} {...rest} />;
}
