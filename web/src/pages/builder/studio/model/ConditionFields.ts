import type { SampleField } from '../../../../api';
import type { JsonLine, SampleEvent } from '../../../../domain';
import { EventPath } from '../../../../domain/routeConfig/StepMapper';
import { FilterFieldCatalog } from '../../model/FilterFieldCatalog';

/** Fields offered by the drop-if condition rows: the current sample's leaves as event paths (`value.x`). */
export class ConditionFields {
  static fields(lines: JsonLine[]): SampleField[] {
    return lines.filter((l) => !l.isBranch).map((l) => ({ path: EventPath.toDocument(l.path), numeric: typeof l.value === 'number' }));
  }

  static catalog(lines: JsonLine[], samples: SampleEvent[]): FilterFieldCatalog {
    return new FilterFieldCatalog(ConditionFields.fields(lines), samples);
  }
}
