import { ToneBadge } from '../../../components/ui';
import { ConnectionDisplay, Tone, type Connection } from '../../../domain';
import { DemoBadgeText } from '../model/ConnectionDrawer';

export type DemoBadgeProps = { connection: Connection };

/** "Demo" pill shown next to demo connections (list row, topic browser); renders nothing for real clusters. */
export function DemoBadge({ connection }: DemoBadgeProps) {
  if (!ConnectionDisplay.isDemo(connection)) return null;
  return (
    <span title={DemoBadgeText.title} data-testid="demo-badge">
      <ToneBadge tone={Tone.Info} dot={false}>
        {DemoBadgeText.label}
      </ToneBadge>
    </span>
  );
}
