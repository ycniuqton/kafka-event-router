import { ApiError, FieldErrorMapper } from '../../../api/http/core';
import { TopicDefaults, type Connection, type TopicInfo, type TopicSettings } from '../../../domain';

/** Texts of the topic browser and the create-topic dialog. */
export const TopicBrowserText = {
  loading: 'Loading topics…',
  empty: 'No topics on this cluster yet.',
  noMatch: 'No topic matches this search.',
  search: 'Search topics',
  showInternal: 'Show internal topics',
  create: '+ Create topic',
  createDisabled: 'Topic creation is off for this connection. Turn on "Allow creating topics" in Edit → Basics.',
  internal: 'internal',
  nameRequired: 'Give the topic a name',
  nameInvalid: 'Use letters, digits, ".", "_" or "-" (max 249 characters)',
  partitionsInvalid: 'Partitions must be between 1 and 1000',
  retentionInvalid: 'Retention must be at least 1 day',
} as const;

/** The create-topic form; blank replication factor = broker default. */
export type TopicForm = { name: string; partitions: string; retentionDays: string; replicationFactor: string };

/** Filtering, create gating and create-topic validation of the topic browser (CN-50, CN-51). */
export class TopicBrowserModel {
  private static readonly errorMapper = new FieldErrorMapper();

  static filter(topics: TopicInfo[], q: string): TopicInfo[] {
    const needle = q.trim().toLowerCase();
    return needle ? topics.filter((t) => t.name.toLowerCase().includes(needle)) : topics;
  }

  /** Mock connections carry no flag and may always create. */
  static canCreate(connection: Connection): boolean {
    return connection.allowTopicCreate ?? true;
  }

  static emptyForm(name = ''): TopicForm {
    return { name, partitions: String(TopicDefaults.partitions), retentionDays: String(TopicDefaults.retentionDays), replicationFactor: '' };
  }

  /** Client-side field errors (the server re-validates). */
  static validate(form: TopicForm): Record<string, string> {
    const errors: Record<string, string> = {};
    const partitions = Number(form.partitions);
    if (!form.name) errors.name = TopicBrowserText.nameRequired;
    else if (!TopicDefaults.namePattern.test(form.name) || form.name.length > TopicDefaults.maxNameLength) errors.name = TopicBrowserText.nameInvalid;
    if (!Number.isInteger(partitions) || partitions < 1 || partitions > TopicDefaults.maxPartitions) errors.partitions = TopicBrowserText.partitionsInvalid;
    if (!(Number(form.retentionDays) >= 1)) errors.retentionMs = TopicBrowserText.retentionInvalid;
    return errors;
  }

  static settings(form: TopicForm): TopicSettings {
    const rf = form.replicationFactor.trim();
    return {
      partitions: Number(form.partitions),
      retentionMs: TopicDefaults.retentionMs(Number(form.retentionDays)),
      ...(rf ? { replicationFactor: Number(rf) } : {}),
    };
  }

  /** Server problems → dialog fields (`/name`, `/partitions`, …); others land on `name`. */
  static serverErrors(error: unknown): Record<string, string> {
    if (error instanceof ApiError) {
      const fields = TopicBrowserModel.errorMapper.map(error);
      if (Object.keys(fields).length) return fields;
    }
    return { name: error instanceof Error ? error.message : String(error) };
  }
}
