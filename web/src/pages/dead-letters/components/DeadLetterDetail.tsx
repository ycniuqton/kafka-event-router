import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { Button, Card, Format, JsonTree, SegmentedControl, ToneBadge } from '../../../components/ui';
import { Can, MaskedHint, useCan } from '../../../access';
import {
  DeadLetterClasses,
  DeadLetterDelivery,
  DeadLetterErrorClassMeta,
  DeadLetterFileFormatLabel,
  DeadLetterResolutionMeta,
  DeadLetterResolutionState,
  Permission,
  Tone,
  type DeadLetter,
  type DeadLetterOriginal,
} from '../../../domain';
import { ErrorClassBadge } from './ErrorClassBadge';
import { ReplayDialog } from './ReplayDialog';
import { useDeadLetterCommands } from '../model/useDeadLetterCommands';
import { useDeadLetterDetail } from '../model/useDeadLetterDetail';
import type { DeadLetterBrowserModel } from '../model/useDeadLetterBrowser';
import { OriginalText, OriginalView } from '../model/DeadLetterTexts';
import { AtLeastOnceNotice } from './AtLeastOnceNotice';
import styles from './DeadLetterDetail.module.css';

export type DeadLetterDetailProps = { deadLetter: DeadLetter; routeName: string; model: DeadLetterBrowserModel; showAllLink: boolean };

/** Selected failure: failing step, stack trace, original event (masked per role) and the fix / retry / restore actions. */
export function DeadLetterDetail({ deadLetter: d, routeName, model, showAllLink }: DeadLetterDetailProps) {
  const { detail, loading } = useDeadLetterDetail(d);
  const original = detail?.original ?? undefined;
  const stack = detail?.stack || d.stack;
  const version = d.versionNo ? ` · v${d.versionNo}` : '';
  const errorClass = DeadLetterClasses.of(d);
  return (
    <Card padding="md" className={styles.detail}>
      <div>
        <div className={styles.title}>
          <ErrorClassBadge errorClass={errorClass} />
          {d.kind} · {d.step}
        </div>
        <div className={styles.meta}>
          {routeName}
          {version} · partition {d.partition} · offset {d.offset} · {Format.monthDayTime(d.failedAt)}
          {d.attempts ? ` · parked after ${d.attempts} auto-retries` : ''}
        </div>
        <div className={styles.meta}>{DeadLetterErrorClassMeta[errorClass].hint}</div>
      </div>
      <ResolutionLine deadLetter={d} />
      <AtLeastOnceNotice show={!d.resolution && DeadLetterDelivery.exactlyOnce([d])} />
      <div className={styles.message}>{d.message}</div>
      {stack && <pre className={styles.stack}>{stack}</pre>}
      <OriginalEvent original={original} loading={loading} />
      <DetailActions deadLetter={d} model={model} showAllLink={showAllLink} original={detail ? original ?? null : undefined} />
    </Card>
  );
}

function ResolutionLine({ deadLetter: d }: { deadLetter: DeadLetter }) {
  if (!d.resolution) return null;
  const meta = DeadLetterResolutionMeta[d.resolution.state];
  return (
    <div className={styles.meta}>
      <ToneBadge tone={meta.tone}>{meta.label}</ToneBadge> by {d.resolution.by} · {Format.monthDayTime(d.resolution.at)}
      {d.resolution.jobId ? ' · background job' : ''}
    </div>
  );
}

/** Original key/value/headers; masked values get the masked badge + hint and no raw view. */
function OriginalEvent({ original, loading }: { original: DeadLetterOriginal | undefined; loading: boolean }) {
  const [view, setView] = useState<OriginalView>(OriginalView.Tree);
  const canRaw = useCan(Permission.SampleReadRaw) && !original?.masked;
  const value = original ? { key: original.key, value: original.value, headers: original.headers } : null;
  return (
    <>
      <div className={styles.sectionRow}>
        <span className={styles.section}>{OriginalText.section(original)}</span>
        {original?.masked && <ToneBadge tone={Tone.Warn}>Masked</ToneBadge>}
        {canRaw && original && <SegmentedControl size="xs" ariaLabel="Original view" options={OriginalText.options} value={view} onChange={setView} />}
      </div>
      {original?.masked && <MaskedHint />}
      <div className={styles.event}>
        {value && (view === OriginalView.Raw && canRaw ? <pre className={styles.raw}>{JSON.stringify(value, null, OriginalText.indent)}</pre> : <JsonTree value={value} />)}
        {!original && <div className={styles.missing}>{loading ? 'Loading the original event…' : 'The original event is no longer available.'}</div>}
      </div>
    </>
  );
}

type DetailActionsProps = Omit<DeadLetterDetailProps, 'routeName'> & { original: DeadLetterOriginal | null | undefined };

function DetailActions({ deadLetter: d, model, showAllLink, original }: DetailActionsProps) {
  const navigate = useNavigate();
  const commands = useDeadLetterCommands();
  const [editing, setEditing] = useState(false);
  const discarded = d.resolution?.state === DeadLetterResolutionState.Discarded;
  return (
    <div className={styles.actions}>
      <Button variant="primary" onClick={() => void commands.openInStudio(d)}>
        Open in Transform Studio
      </Button>
      <Can permission={Permission.DlqAct}>
        {!d.resolution && <Button onClick={() => void model.retry([d.id])}>Retry</Button>}
        {!d.resolution && (
          <Button disabled={original === undefined} onClick={() => setEditing(true)}>
            Edit &amp; replay…
          </Button>
        )}
        {!d.resolution && <Button onClick={() => void model.discard([d.id])}>Discard</Button>}
        {discarded && <Button onClick={() => void model.restore([d.id])}>Restore</Button>}
        <Button onClick={() => void commands.exportJson(d.routeId, { ids: [d.id] }, model.format)}>Export {DeadLetterFileFormatLabel[model.format]}</Button>
      </Can>
      {editing && original !== undefined && <ReplayDialog deadLetter={d} original={original} onClose={() => setEditing(false)} onReplayed={model.resolved} />}
      {showAllLink && (
        <Button variant="ghost" onClick={() => navigate(AppPaths.deadLetters())}>
          All dead letters →
        </Button>
      )}
    </div>
  );
}
