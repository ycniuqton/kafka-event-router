import { useEffect, useState } from 'react';
import { services } from '../../../../api';
import { LibraryKind, LibraryRules, StepLine, type LibraryVersion } from '../../../../domain';
import type { StepSettingsProps } from '../components/settings/StepSettingsProps';
import { StepChangeLabel } from '../model/StudioText';
import { LibraryPicker } from './LibraryPicker';
import styles from './Library.module.css';

/** Texts of the library step form. */
class LibraryStepText {
  static readonly hint = 'Runs the pinned version as one step. A newer version never replaces it automatically.';
  static readonly steps = (version: LibraryVersion) => `${LibraryRules.versionLabel(version.version)} · ${LibraryRules.stepsLabel(version.size)}${version.changeNote ? ` · ${version.changeNote}` : ''}`;
}

/** Library transform step: pick a transform and the version it is pinned to; shows that version's steps. */
export function LibraryStepSettings({ step, update }: StepSettingsProps) {
  const [loaded, setLoaded] = useState<{ pin: string; version: LibraryVersion | null } | null>(null);
  const label = StepChangeLabel.edited(step.kind);
  const pin = step.libraryRef && step.libraryVersion ? LibraryRules.pinLabel(step.libraryRef, step.libraryVersion) : '';
  useEffect(() => {
    if (!step.libraryRef || !step.libraryVersion) return;
    let live = true;
    const [ref, pinned, key] = [step.libraryRef, step.libraryVersion, LibraryRules.pinLabel(step.libraryRef, step.libraryVersion)];
    services.library
      .list({ names: [ref] })
      .then((items) => (items[0] ? services.library.version(LibraryKind.Transform, items[0].id, pinned) : null))
      .then((v) => live && setLoaded({ pin: key, version: v }), () => undefined);
    return () => {
      live = false;
    };
  }, [step.libraryRef, step.libraryVersion]);
  const version = loaded?.pin === pin ? loaded.version : null;
  return (
    <div className={styles.settings}>
      <LibraryPicker kind={LibraryKind.Transform} name={step.libraryRef ?? ''} version={step.libraryVersion}
        onChange={(libraryRef, libraryVersion) => update({ libraryRef, libraryVersion }, label)} />
      <span className={styles.muted}>{LibraryStepText.hint}</span>
      {version && (
        <div className={styles.preview} aria-label="Pinned steps">
          <span className={styles.muted}>{LibraryStepText.steps(version)}</span>
          {(version.steps ?? []).map((s) => (
            <span key={s.id}>{StepLine.of(s)}</span>
          ))}
        </div>
      )}
    </div>
  );
}
