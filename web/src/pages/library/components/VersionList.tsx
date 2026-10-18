import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonSize, ButtonVariant, Format, Mono } from '../../../components/ui';
import { LibraryKind, LibraryRules, StepLine, Tone, type LibraryVersion } from '../../../domain';
import { useNow, useServiceQuery } from '../../../store';
import styles from './LibraryList.module.css';

/** Texts of the version list. */
class VersionText {
  static readonly show = 'Show';
  static readonly hide = 'Hide';
  static readonly more = (n: number) => `… ${n} more rows`;
  static size(kind: LibraryKind, size: number): string {
    return kind === LibraryKind.Transform ? LibraryRules.stepsLabel(size) : `${size.toLocaleString()} rows`;
  }
}

/** Immutable versions, newest first, with author, change note, hash and the content on demand. */
export function VersionList({ kind, id, refresh }: { kind: LibraryKind; id: string; refresh: number }) {
  const { data: versions, error } = useServiceQuery(() => services.library.versions(kind, id), [kind, id, refresh]);
  const now = useNow();
  if (error) return <div className={styles.error}>{error.message}</div>;
  return (
    <div className={styles.list} aria-label="Versions">
      {(versions ?? []).map((v) => (
        <div key={v.version} className={styles.item}>
          <div className={styles.itemHead}>
            <Mono tone={Tone.Accent}>{LibraryRules.versionLabel(v.version)}</Mono>
            <span>{VersionText.size(kind, v.size)}</span>
            <span className={styles.muted}>{v.createdBy} · {Format.relativeTime(v.createdAt, now)}</span>
            <Mono tone={Tone.Muted} truncate>{v.configHash}</Mono>
          </div>
          {v.changeNote && <span className={styles.secondary}>{v.changeNote}</span>}
          <VersionContent kind={kind} id={id} version={v} />
        </div>
      ))}
    </div>
  );
}

/** Loads and shows the steps (or first rows) of one version. */
function VersionContent({ kind, id, version }: { kind: LibraryKind; id: string; version: LibraryVersion }) {
  const [content, setContent] = useState<LibraryVersion | null>(null);
  const [open, setOpen] = useState(false);
  const toggle = async () => {
    if (!open && !content) setContent(await services.library.version(kind, id, version.version));
    setOpen(!open);
  };
  const lines = content?.steps?.map((s) => StepLine.of(s)) ?? content?.rows?.map((r) => `${r.key} → ${JSON.stringify(r.value)}`) ?? [];
  return (
    <>
      <div>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => void toggle()}>{open ? VersionText.hide : VersionText.show}</Button>
      </div>
      {open && (
        <div className={styles.code}>
          {lines.map((l, i) => <span key={`${i}-${l}`}>{l}</span>)}
          {content?.rows && version.size > content.rows.length && <span>{VersionText.more(version.size - content.rows.length)}</span>}
        </div>
      )}
    </>
  );
}
