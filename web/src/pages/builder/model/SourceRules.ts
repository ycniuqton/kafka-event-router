import { ConnectionStatusMeta, SourceFormatType, StartPosition, type Connection, type DraftSource, type TopicInfo } from '../../../domain';

/** Topic search of the Source step (name fragment or regex) and the regex subscription preview. */
export class TopicPattern {
  /** Topics a subscription pattern matches now (Kafka re-checks new topics while running); null for a bad regex. */
  static matches(topics: TopicInfo[], pattern: string): TopicInfo[] | null {
    const regex = TopicPattern.compile(pattern.trim());
    if (!regex) return null;
    return topics.filter((t) => regex.test(t.name));
  }

  static filter(topics: TopicInfo[], query: string): TopicInfo[] {
    const q = query.trim().toLowerCase();
    if (!q) return topics;
    const regex = TopicPattern.compile(q);
    return topics.filter((t) => t.name.includes(q) || !!regex?.test(t.name));
  }

  private static compile(query: string): RegExp | null {
    try {
      return new RegExp(query);
    } catch {
      return null;
    }
  }
}

/** What the source subscribes to: listed topics, or `/pattern/` (P2). */
export class SourceSubscription {
  static readonly none = '—';

  static isSet(source: DraftSource): boolean {
    return !!source.topicPattern?.trim() || source.topics.length > 0;
  }

  static text(source: DraftSource, joiner: string): string {
    const pattern = source.topicPattern?.trim();
    if (pattern) return `/${pattern}/`;
    return source.topics.join(joiner) || SourceSubscription.none;
  }
}

/** Source value formats (P2 override); Avro and Protobuf read through the connection's schema registry. */
export class SourceFormatOptions {
  static readonly options: { value: SourceFormatType; label: string }[] = [
    { value: SourceFormatType.Json, label: 'JSON (auto-detected)' },
    { value: SourceFormatType.Avro, label: 'Avro (schema registry)' },
    { value: SourceFormatType.Protobuf, label: 'Protobuf (schema registry)' },
    { value: SourceFormatType.String, label: 'String' },
    { value: SourceFormatType.Bytes, label: 'Bytes' },
  ];
  static readonly registryWarning = 'This connection has no schema registry; add one in Connections to read this format.';

  static usesRegistry(type: SourceFormatType): boolean {
    return type === SourceFormatType.Avro || type === SourceFormatType.Protobuf;
  }

  /** Default subject of a topic's values (Confluent TopicNameStrategy). */
  static defaultSubject(topic: string | undefined): string {
    return topic ? `${topic}-value` : '';
  }
}

/** Start offsets per partition: one row per partition of the single picked topic (not with a pattern), plus stored ones. */
export class OffsetRows {
  static readonly maxRows = 64;

  static partitions(source: DraftSource, topics: TopicInfo[]): string[] {
    const stored = Object.keys(source.offsets ?? {});
    const single = !source.topicPattern?.trim() && source.topics.length === 1;
    const topic = single ? topics.find((t) => t.name === source.topics[0]) : undefined;
    const count = Math.min(topic?.partitions ?? 0, OffsetRows.maxRows);
    const known = Array.from({ length: count }, (_, i) => String(i));
    return [...new Set([...known, ...stored])].sort((a, b) => Number(a) - Number(b));
  }

  /** Sets (or clears, for an empty input) one partition's offset. */
  static set(offsets: Record<string, number> | undefined, partition: string, raw: string): Record<string, number> {
    const next = { ...(offsets ?? {}) };
    const n = Number(raw);
    if (raw.trim() === '' || !Number.isInteger(n) || n < 0) delete next[partition];
    else next[partition] = n;
    return next;
  }
}

/** Consumer group: the stored one, else the server default `router.<route-name>` (internal/routes/naming.go). */
export class ConsumerGroupName {
  static readonly prefix = 'router.';
  static readonly fallback = 'route';
  private static readonly invalid = /[^a-z0-9._-]+/g;
  private static readonly edges = /^-+|-+$/g;

  static of(routeName: string): string {
    const slug = (routeName.trim() || ConsumerGroupName.fallback).toLowerCase().replace(ConsumerGroupName.invalid, '-').replace(ConsumerGroupName.edges, '');
    return `${ConsumerGroupName.prefix}${slug}`;
  }

  static effective(source: DraftSource, routeName: string): string {
    return source.consumerGroup?.trim() || ConsumerGroupName.of(routeName);
  }
}

/** Start-from choices (specific offsets are P2) and the date input format. */
export class StartFromOptions {
  static readonly positions: StartPosition[] = [StartPosition.Latest, StartPosition.Earliest, StartPosition.DateTime, StartPosition.Offsets];
  static readonly inputLength = 16;

  /** ISO instant → `datetime-local` value (UTC, minutes). */
  static toInput(iso: string | undefined): string {
    return (iso ?? new Date().toISOString()).slice(0, StartFromOptions.inputLength);
  }

  /** `datetime-local` value (read as UTC) → ISO instant, or undefined when empty/invalid. */
  static fromInput(value: string): string | undefined {
    const at = new Date(`${value}Z`);
    return value && !Number.isNaN(at.getTime()) ? at.toISOString() : undefined;
  }
}

/** Connection dropdown label: name (REST) or id (mock) and health, e.g. `orders-prod  ·  healthy`; names by id for summaries. */
export class ConnectionOption {
  static label(connection: Connection): string {
    return `${connection.name ?? connection.id}  ·  ${ConnectionStatusMeta[connection.status].label.toLowerCase()}`;
  }

  /** Display name of a connection id among the loaded connections (falls back to the id). */
  static name(connections: Connection[], id: string): string {
    return connections.find((c) => c.id === id)?.name ?? id;
  }
}
