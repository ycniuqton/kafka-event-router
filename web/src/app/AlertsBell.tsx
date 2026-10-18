import { useEffect, useRef, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { Button, Format, HealthDot } from '../components/ui';
import type { Alert } from '../domain';
import { actions, Keys, useAlerts, useAlertsUnread, useNow, useShortcut } from '../store';
import styles from './AlertsBell.module.css';
import { useIntentRunner } from './useIntentRunner';

/** Copy of the bell. */
const AlertsBellText = {
  button: 'Alerts',
  empty: 'No alerts. Rules you add in a route’s Settings tab notify you here.',
  markAll: 'Mark all read',
  ack: 'Ack',
  acked: 'Acknowledged',
} as const;

/** Top-bar bell: unread badge, dropdown of firing and recent alerts; a row opens its intent and acknowledges it. */
export function AlertsBell() {
  const alerts = useAlerts();
  const unread = useAlertsUnread();
  const location = useLocation();
  const [openAt, setOpenAt] = useState<string | null>(null);
  const open = openAt === location.key;
  const wrapRef = useRef<HTMLDivElement>(null);
  const runIntent = useIntentRunner();
  useShortcut({ key: Keys.Escape, allowInInputs: true, handler: () => setOpenAt(null) }, open);
  useEffect(() => {
    if (!open) return undefined;
    void actions.monitoring.refreshAlerts();
    const onDown = (e: MouseEvent) => !wrapRef.current?.contains(e.target as Node) && setOpenAt(null);
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, [open]);
  const choose = (alert: Alert) => {
    setOpenAt(null);
    void actions.monitoring.ack(alert.id);
    runIntent(alert.intent);
  };
  return (
    <div className={styles.wrap} ref={wrapRef}>
      <Button size="lg" onClick={() => setOpenAt(open ? null : location.key)} aria-expanded={open} aria-haspopup="menu">
        {AlertsBellText.button} <span className={styles.count} data-zero={unread === 0 || undefined}>{unread}</span>
      </Button>
      {open && <AlertsDropdown alerts={alerts} unread={unread} onChoose={choose} />}
    </div>
  );
}

type AlertsDropdownProps = { alerts: Alert[]; unread: number; onChoose: (alert: Alert) => void };

/** The open menu: header with "Mark all read", then one row per alert. */
function AlertsDropdown({ alerts, unread, onChoose }: AlertsDropdownProps) {
  const now = useNow();
  return (
    <div className={styles.menu} role="menu">
      <div className={styles.header}>
        <span className={styles.heading}>{AlertsBellText.button}</span>
        {unread > 0 && (
          <Button size="sm" variant="ghost" onClick={() => void actions.monitoring.ackAll()}>
            {AlertsBellText.markAll}
          </Button>
        )}
      </div>
      <div className={styles.list}>
        {alerts.length === 0 && <div className={styles.empty}>{AlertsBellText.empty}</div>}
        {alerts.map((a) => (
          <div key={a.id} className={styles.row} data-unread={!a.acked || undefined}>
            <button type="button" role="menuitem" className={styles.item} onClick={() => onChoose(a)}>
              <span className={styles.dot}>
                <HealthDot tone={a.tone} />
              </span>
              <span className={styles.text}>
                <span className={styles.title}>{a.title}</span>
                <span className={styles.detail}>{a.detail}</span>
                {a.at && <span className={styles.time}>{Format.relativeTime(a.at, now)}</span>}
              </span>
            </button>
            {!a.acked && (
              <Button size="sm" variant="ghost" className={styles.ack} aria-label={`${AlertsBellText.ack} ${a.title}`} onClick={() => void actions.monitoring.ack(a.id)}>
                {AlertsBellText.ack}
              </Button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
