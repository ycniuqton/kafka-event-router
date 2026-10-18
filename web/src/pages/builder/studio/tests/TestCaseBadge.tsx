import { ToneBadge } from '../../../../components/ui';
import { Tone, TestVerdictLabel, TestVerdictTone, type TestCaseResult } from '../../../../domain';
import { TestCaseText } from './TestCaseText';

/** Pass / fail / error badge of one case (grey until the case has run). */
export function TestCaseBadge({ result }: { result: TestCaseResult | undefined }) {
  if (!result) return <ToneBadge tone={Tone.Off}>{TestCaseText.pending}</ToneBadge>;
  return <ToneBadge tone={TestVerdictTone[result.result]}>{TestVerdictLabel[result.result]}</ToneBadge>;
}
