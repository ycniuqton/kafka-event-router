import kafkajs, { type Admin, type IHeaders } from 'kafkajs';
import SnappyCodec from 'kafkajs-snappy';
import { Profile } from './Target';

const { CompressionCodecs, CompressionTypes, Kafka, logLevel } = kafkajs;

/** Workers compress output batches with snappy. */
CompressionCodecs[CompressionTypes.Snappy] = SnappyCodec;

/** Timing of runner-side Kafka checks. */
export const KafkaTiming = { consumeMs: 60_000, quietMs: 8_000, groupDeleteAttempts: 15, groupDeleteWaitMs: 1_000 } as const;

/** A record as the test runner sees it. */
export type AccRecord = { key: string | null; value: any; headers: Record<string, string>; partition: number; offset: string };

/** A record to produce. */
export type AccMessage = { key?: string | null; value: unknown; headers?: Record<string, string>; partition?: number };

/** Kafka access of the test runner itself (through the dev tunnel; same cluster as the platform). */
export class AccKafka {
  private readonly kafka = new Kafka({ clientId: 'acc-runner', brokers: [Profile.runnerBootstrap], logLevel: logLevel.NOTHING });

  async produce(topic: string, messages: AccMessage[]): Promise<void> {
    const producer = this.kafka.producer({ allowAutoTopicCreation: false });
    await producer.connect();
    try {
      await producer.send({ topic, messages: messages.map(AccKafka.encode) });
    } finally {
      await producer.disconnect();
    }
  }

  /** Raw bytes (e.g. invalid JSON) to a topic. */
  async produceRaw(topic: string, values: string[]): Promise<void> {
    const producer = this.kafka.producer({ allowAutoTopicCreation: false });
    await producer.connect();
    try {
      await producer.send({ topic, messages: values.map((value) => ({ value })) });
    } finally {
      await producer.disconnect();
    }
  }

  /** Reads from the beginning until `count` records arrived or the timeout passed (read_committed by default). */
  async consume(topic: string, count: number, timeoutMs: number = KafkaTiming.consumeMs, readUncommitted = false): Promise<AccRecord[]> {
    const groupId = `acc-verify-${topic}-${Date.now()}`;
    const consumer = this.kafka.consumer({ groupId, readUncommitted });
    const out: AccRecord[] = [];
    await consumer.connect();
    try {
      await consumer.subscribe({ topic, fromBeginning: true });
      await this.collect(consumer, out, count, timeoutMs);
    } finally {
      await consumer.disconnect();
      await this.deleteGroups([groupId]);
    }
    return out;
  }

  /** Waits a quiet period and returns everything on the topic (for "no more than N" checks). */
  async drain(topic: string, quietMs: number = KafkaTiming.quietMs): Promise<AccRecord[]> {
    return this.consume(topic, Number.MAX_SAFE_INTEGER, quietMs);
  }

  async topicExists(topic: string): Promise<boolean> {
    return this.withAdmin(async (admin) => (await admin.listTopics()).includes(topic));
  }

  async deleteTopics(topics: string[]): Promise<void> {
    const existing = await this.withAdmin((admin) => admin.listTopics());
    const doomed = topics.filter((t) => existing.includes(t));
    if (doomed.length) await this.withAdmin((admin) => admin.deleteTopics({ topics: doomed }).catch(() => undefined));
  }

  /** Deletes consumer groups, retrying while a paused runner is still leaving them. */
  async deleteGroups(groups: string[]): Promise<void> {
    await this.withAdmin(async (admin) => {
      for (const group of groups) await AccKafka.deleteGroup(admin, group);
    });
  }

  /** Consumer groups (other than the runner's own verify groups) that committed offsets on topic. */
  async groupsReading(topic: string): Promise<string[]> {
    return this.withAdmin(async (admin) => {
      const groups = (await admin.listGroups()).groups.map((g) => g.groupId).filter((g) => !g.startsWith('acc-verify-'));
      const owners: string[] = [];
      for (const groupId of groups) {
        const offsets = await admin.fetchOffsets({ groupId, topics: [topic] }).catch(() => []);
        if (offsets.some((t) => t.partitions.some((p) => Number(p.offset) >= 0))) owners.push(groupId);
      }
      return owners;
    });
  }

  /** Consumer groups whose id starts with one of the prefixes (cleanup of leftovers). */
  async groupsWithPrefixes(prefixes: string[]): Promise<string[]> {
    return this.withAdmin(async (admin) => (await admin.listGroups()).groups.map((g) => g.groupId).filter((g) => prefixes.some((p) => g.startsWith(p))));
  }

  /** Topic names starting with prefix (cleanup of leftovers). */
  async topicsWithPrefix(prefix: string): Promise<string[]> {
    return this.withAdmin(async (admin) => (await admin.listTopics()).filter((t) => t.startsWith(prefix)));
  }

  private async collect(consumer: ReturnType<InstanceType<typeof Kafka>['consumer']>, out: AccRecord[], count: number, timeoutMs: number): Promise<void> {
    let resolveDone: () => void = () => undefined;
    const done = new Promise<void>((resolve) => (resolveDone = resolve));
    await consumer.run({
      eachMessage: async ({ message, partition }) => {
        out.push({ key: message.key?.toString() ?? null, value: AccKafka.decode(message.value), headers: AccKafka.text(message.headers), partition, offset: message.offset });
        if (out.length >= count) resolveDone();
      },
    });
    await Promise.race([done, new Promise((r) => setTimeout(r, timeoutMs))]);
  }

  private async withAdmin<T>(fn: (admin: Admin) => Promise<T>): Promise<T> {
    const admin = this.kafka.admin();
    await admin.connect();
    try {
      return await fn(admin);
    } finally {
      await admin.disconnect();
    }
  }

  private static async deleteGroup(admin: Admin, group: string): Promise<void> {
    for (let attempt = 0; attempt < KafkaTiming.groupDeleteAttempts; attempt++) {
      const deleted = await admin.deleteGroups([group]).then(() => true, () => false);
      const known = (await admin.listGroups()).groups.some((g) => g.groupId === group);
      if (deleted || !known) return;
      await new Promise((r) => setTimeout(r, KafkaTiming.groupDeleteWaitMs));
    }
  }

  private static encode(m: AccMessage): { key?: string | null; value: string; headers?: Record<string, string>; partition?: number } {
    return { key: m.key ?? null, value: typeof m.value === 'string' ? m.value : JSON.stringify(m.value), headers: m.headers, partition: m.partition };
  }

  private static decode(value: Buffer | null): any {
    const text = value?.toString() ?? 'null';
    try {
      return JSON.parse(text);
    } catch {
      return text;
    }
  }

  private static text(headers: IHeaders | undefined): Record<string, string> {
    return Object.fromEntries(Object.entries(headers ?? {}).map(([k, v]) => [k, String(Array.isArray(v) ? v[0] : v)]));
  }
}
