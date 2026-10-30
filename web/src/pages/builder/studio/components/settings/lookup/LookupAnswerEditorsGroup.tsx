import type { LookupSourceStep } from '../../../../../../domain';
import { LookupMapEditor, LookupMockEditor, LookupPolicyEditor } from './LookupAnswerEditors';

/** Mapping, policies and the preview mock of a lookup source step, in reading order. */
export function LookupAnswerEditors({ step, onChange }: { step: LookupSourceStep; onChange(next: LookupSourceStep): void }) {
  return (
    <>
      <LookupMapEditor step={step} onChange={onChange} />
      <LookupPolicyEditor step={step} onChange={onChange} />
      <LookupMockEditor step={step} onChange={onChange} />
    </>
  );
}
