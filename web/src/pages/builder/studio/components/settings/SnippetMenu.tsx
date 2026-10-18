import { useCallback, useRef, useState } from 'react';
import { Button, ButtonSize, ButtonVariant } from '../../../../../components/ui';
import { useDismiss } from '../../hooks/useDismiss';
import { JsSnippetCatalog, type JsSnippet } from '../../model/JsSnippets';
import styles from './SnippetMenu.module.css';

/** "Snippets" dropdown inserting common patterns into the JS step. */
export function SnippetMenu({ onInsert }: { onInsert(snippet: JsSnippet): void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(ref, open, close);
  return (
    <div className={styles.wrap} ref={ref}>
      <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)}>
        Snippets ▾
      </Button>
      {open && (
        <div className={styles.menu} role="menu">
          {JsSnippetCatalog.all.map((snippet) => (
            <button key={snippet.id} type="button" role="menuitem" className={styles.item} onClick={() => {
                close();
                onInsert(snippet);
              }}>
              {snippet.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
