import type { AssistContext } from '../../../../api';
import type { StudioModel } from '../hooks/StudioContext';

/** At most two sample events sent with a proposal, only when the connection is known (the server masks them). */
export class AssistSamples {
  static readonly max = 2;

  static of(model: StudioModel): Pick<AssistContext, 'samples' | 'connectionId'> {
    if (!model.connectionId) return {};
    const samples = model.samples.slice(0, AssistSamples.max).map((s) => ({ key: s.key, value: s.value }));
    return { samples, connectionId: model.connectionId };
  }
}
