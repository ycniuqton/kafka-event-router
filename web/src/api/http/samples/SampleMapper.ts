import type { SampleEvent } from '../../../domain';
import { SampleRecords } from '../../../domain/preview';
import type { SampleField } from '../../contracts';
import type { FieldSchemaDto, SampleDto } from '../preview/PreviewDtos';

/** Converts preview DTOs into the screens' domain shapes (rules live in domain `SampleRecords`). */
export class SampleMapper {
  static event(dto: SampleDto): SampleEvent {
    return SampleRecords.toEvent(dto);
  }

  static events(dtos: SampleDto[]): SampleEvent[] {
    return SampleRecords.toEvents(dtos);
  }

  static fields(schema: FieldSchemaDto): SampleField[] {
    return SampleRecords.fields(schema);
  }
}
