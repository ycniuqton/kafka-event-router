import { useState } from 'react';
import { Mono, Tabs } from '../../../components/ui';
import { LibraryKind, LibraryRules, Tone, type LibraryTransform } from '../../../domain';
import { DetailTab, LibraryText } from '../model/LibraryLayout';
import { MetadataSection } from './MetadataSection';
import { TransformTestCases } from './TransformTestCases';
import { UsageList } from './UsageList';
import { VersionList } from './VersionList';
import styles from './LibraryList.module.css';

type TransformDetailProps = { transform: LibraryTransform; onChange(t: LibraryTransform): void; onArchived(): void };

/** Drawer body of a library transform: metadata, versions, usages (routes per version) and test cases. */
export function TransformDetail({ transform, onChange, onArchived }: TransformDetailProps) {
  const [tab, setTab] = useState<DetailTab>(DetailTab.Versions);
  return (
    <div className={styles.detail}>
      <div className={styles.meta}>
        <Mono tone={Tone.Accent}>{transform.version}</Mono>
        <span>{LibraryRules.stepsLabel(transform.size)}</span>
        <span>{LibraryRules.usageLabel(transform.usedBy)}</span>
        <span className={styles.muted}>{transform.team}</span>
      </div>
      <MetadataSection kind={LibraryKind.Transform} item={transform} onChange={onChange} onArchived={onArchived} />
      <Tabs items={LibraryText.transformTabs} value={tab} onChange={setTab} />
      {tab === DetailTab.Versions && <VersionList kind={LibraryKind.Transform} id={transform.id} refresh={transform.latestVersion} />}
      {tab === DetailTab.Usages && <UsageList kind={LibraryKind.Transform} id={transform.id} />}
      {tab === DetailTab.TestCases && <TransformTestCases transformId={transform.id} />}
    </div>
  );
}
