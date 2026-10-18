import type { Activity, CurrentUser, Member, SampleEvent } from '../../domain';
import { CURRENT_USER, MEMBERS, SAMPLES, SampleSchema } from '../../mocks';
import {
  ActivityService,
  MembersService,
  SamplesService,
  type SampleField,
} from '../contracts';
import type { MockBackend } from './MockBackend';

/** Read-only mock services backed by static seed data. */
export class MockActivityService extends ActivityService {
  private readonly backend: MockBackend;
  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }
  list(): Promise<Activity[]> {
    return this.backend.reply(this.backend.db.activity);
  }
}

/** Every topic serves the same 24 `order-events` samples, as in the design. */
export class MockSamplesService extends SamplesService {
  private readonly backend: MockBackend;
  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }
  list(): Promise<SampleEvent[]> {
    return this.backend.reply(SAMPLES);
  }
  peek(_connectionId: string, _topic: string, count: number): Promise<SampleEvent[]> {
    return this.backend.reply(SAMPLES.slice(-count).reverse());
  }
  fields(): Promise<SampleField[]> {
    return this.backend.reply(SampleSchema.fields.map((path) => ({ path, numeric: SampleSchema.isNumeric(path) })));
  }
}

export class MockMembersService extends MembersService {
  private readonly backend: MockBackend;
  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }
  list(): Promise<Member[]> {
    return this.backend.reply(MEMBERS);
  }
  current(): Promise<CurrentUser> {
    return this.backend.reply(CURRENT_USER);
  }
}
