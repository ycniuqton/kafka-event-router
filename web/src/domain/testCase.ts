import type { KafkaEvent } from './event';
import type { JsonValue } from './json';
import { EventStatus, type InlineEvent, type PatchOp } from './preview/types';
import { Tone } from './tone';

/** Verdict of one saved test case in a run (Go `testcases.CaseResult`). */
export const TestVerdict = { Pass: 'pass', Fail: 'fail', Error: 'error' } as const;
export type TestVerdict = (typeof TestVerdict)[keyof typeof TestVerdict];

/** Locked output of a passed case: one output `{key, value, headers}`, an array of several, or null. */
export type TestCaseOutput = JsonValue;

/** A saved test case of a route branch: a pinned input event and its locked outcome (spec §8.5). */
export type TestCase = {
  id: string;
  routeId: string;
  branchId: string;
  name: string;
  inputEvent: InlineEvent;
  expectedOutcome: EventStatus;
  expectedOutput: TestCaseOutput;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  /** Row version, sent back as If-Match on updates. */
  version: number;
};

export type NewTestCase = Pick<TestCase, 'name' | 'inputEvent' | 'expectedOutcome' | 'expectedOutput'> & { branchId?: string };

export type TestCasePatch = Partial<Pick<TestCase, 'name' | 'expectedOutcome' | 'expectedOutput'>>;

/** One verdict of a run; `diff` turns the expected output into the actual one (RFC 6902). */
export type TestCaseResult = {
  caseId: string;
  name: string;
  branchId: string;
  result: TestVerdict;
  actualOutcome: EventStatus;
  actualOutput: TestCaseOutput;
  diff: PatchOp[];
  error?: string;
};

export type TestCaseRunReport = { results: TestCaseResult[]; passed: number; failed: number };

export const TestVerdictTone: Record<TestVerdict, Tone> = {
  [TestVerdict.Pass]: Tone.Ok,
  [TestVerdict.Fail]: Tone.Err,
  [TestVerdict.Error]: Tone.Warn,
};

export const TestVerdictLabel: Record<TestVerdict, string> = {
  [TestVerdict.Pass]: 'Pass',
  [TestVerdict.Fail]: 'Fail',
  [TestVerdict.Error]: 'Error',
};

export const TestOutcomeLabel: Record<EventStatus, string> = {
  [EventStatus.Passed]: 'Output',
  [EventStatus.Filtered]: 'Filtered out',
  [EventStatus.Dropped]: 'Dropped',
  [EventStatus.Error]: 'Error',
};

/** Limits of saved cases (Go `testcases.DefaultLimits`). */
export const TestCaseLimits = { maxCases: 200, maxEventBytes: 256 * 1024 } as const;

/** Converts Studio events into the documents a test case stores (key text '' ↔ null, headers list ↔ map). */
export class TestCaseDocuments {
  /** The pinned input of a sample (engine event JSON shape). */
  static input(event: KafkaEvent & { topic?: string; timestamp?: string }): InlineEvent {
    const headers = Object.entries(event.headers ?? {}).map(([key, value]) => ({ key, value }));
    const out: InlineEvent = { key: event.key === '' ? null : event.key, value: event.value as JsonValue, headers };
    if (event.topic) out.topic = event.topic;
    if (event.timestamp) out.timestamp = event.timestamp;
    return out;
  }

  /** The Studio event a pinned input stands for. */
  static event(input: InlineEvent): KafkaEvent {
    const key = input.key === null || input.key === undefined ? '' : typeof input.key === 'string' ? input.key : JSON.stringify(input.key);
    const headers = Object.fromEntries((input.headers ?? []).map((h) => [h.key, h.value ?? '']));
    const value = input.value !== null && typeof input.value === 'object' && !Array.isArray(input.value) ? input.value : {};
    return { key, headers, value };
  }

  /** The locked output of one event (null when the event has no output). */
  static output(event: KafkaEvent | null | undefined): TestCaseOutput {
    if (!event) return null;
    return { key: event.key === '' ? null : event.key, value: event.value as JsonValue, headers: { ...event.headers } };
  }
}
