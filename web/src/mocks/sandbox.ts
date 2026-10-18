import type { RuntimeInfo } from '../domain';

/** Runtime info as served by the API (`GET /sandbox/runtime-info`, default settings). */
export const SANDBOX_RUNTIME_INFO: RuntimeInfo = {
  "engine": "quickjs_wazero",
  "determinismVersion": 1,
  "limits": {
    "defaults": {
      "jsTimeoutMs": 50,
      "jsMemoryMb": 64,
      "jsMaxOutputKb": 1024,
      "logKbPerCall": 16
    },
    "maxima": {
      "jsTimeoutMs": 1000,
      "jsMemoryMb": 256,
      "jsMaxOutputKb": 8192,
      "logKbPerCall": null
    }
  },
  "helpers": [
    {
      "name": "mask.email",
      "signature": "(value: unknown) => string",
      "doc": "Masks an email but its first letter and the domain, e.g. j***@x.com."
    },
    {
      "name": "mask.last4",
      "signature": "(value: unknown) => string",
      "doc": "Masks everything except the last 4 characters."
    },
    {
      "name": "mask.full",
      "signature": "(value: unknown) => string",
      "doc": "Replaces every character with a mask symbol."
    },
    {
      "name": "hash",
      "signature": "(value: unknown) => string",
      "doc": "Stable salted SHA-256 hash (hex); the salt is per team and never visible to code."
    },
    {
      "name": "now",
      "signature": "() => string",
      "doc": "Processing time of this event as an ISO-8601 UTC string; frozen for the whole call."
    },
    {
      "name": "formatDate",
      "signature": "(value: string | number, layout: string, tz?: string) => string",
      "doc": "Formats an ISO string or epoch milliseconds in a named layout (rfc3339, rfc3339_ms, date, datetime) and IANA zone."
    },
    {
      "name": "lookup",
      "signature": "(table: string, key: unknown) => unknown",
      "doc": "Looks up a key in a lookup table of this route (library tables at the version pinned by the route); returns null when the table or key is missing."
    },
    {
      "name": "log",
      "signature": "(...values: unknown[]) => void",
      "doc": "Writes a line to the preview Console tab; output is size-capped and never stored in production."
    }
  ],
  "typings": "/** Integer beyond \u00b12^53 on Avro/Protobuf routes, as decimal digits (use BigInt to compute). */\ntype RouterLong = { $long: string };\n/** Binary data (Avro/Protobuf bytes, non-UTF-8 headers), base64-encoded. */\ntype RouterBytes = { $bytes: string };\n/** Exact decimal on Avro/Protobuf routes, e.g. \"12.34\". */\ntype RouterDecimal = { $decimal: string };\n/** Timestamp on Avro/Protobuf routes, RFC 3339 in UTC. */\ntype RouterTimestamp = { $timestamp: string };\n/** Lossless tagged values of Avro/Protobuf routes; return them unchanged or build new ones. */\ntype RouterTagged = RouterLong | RouterBytes | RouterDecimal | RouterTimestamp;\ntype RouterHeaderValue = string | RouterBytes;\ninterface RouterEvent {\n  key: unknown;\n  value: any;\n  headers: Record<string, RouterHeaderValue>;\n  topic: string;\n  partition: number;\n  offset: number;\n  timestamp: string;\n}\ninterface RouterOutput {\n  key?: unknown;\n  value?: unknown;\n  headers?: Record<string, RouterHeaderValue>;\n}\ninterface RouterCtx {\n  /** Stable salted SHA-256 hash (hex); the salt is per team and never visible to code. */\n  hash: (value: unknown) => string;\n  /** Processing time of this event as an ISO-8601 UTC string; frozen for the whole call. */\n  now: () => string;\n  /** Formats an ISO string or epoch milliseconds in a named layout (rfc3339, rfc3339_ms, date, datetime) and IANA zone. */\n  formatDate: (value: string | number, layout: string, tz?: string) => string;\n  /** Looks up a key in a lookup table of this route (library tables at the version pinned by the route); returns null when the table or key is missing. */\n  lookup: (table: string, key: unknown) => unknown;\n  /** Writes a line to the preview Console tab; output is size-capped and never stored in production. */\n  log: (...values: unknown[]) => void;\n  mask: {\n    /** Masks an email but its first letter and the domain, e.g. j***@x.com. */\n    email: (value: unknown) => string;\n    /** Masks everything except the last 4 characters. */\n    last4: (value: unknown) => string;\n    /** Replaces every character with a mask symbol. */\n    full: (value: unknown) => string;\n  };\n}\ndeclare const ctx: RouterCtx;\ndeclare function transform(event: RouterEvent, ctx: RouterCtx): RouterOutput | RouterOutput[] | null;\n",
  "safetyRules": [
    {
      "id": "isolation",
      "text": "Code runs in an isolated sandbox: no network calls, no file access, no access to credentials or other routes."
    },
    {
      "id": "limits",
      "text": "Time limit per event and memory limit per route; exceeding them sends the event to the dead-letter topic and shows a warning on the step."
    },
    {
      "id": "determinism",
      "text": "No randomness or current time except through ctx, so test cases are repeatable (Math.random and Date are seeded and frozen per event)."
    },
    {
      "id": "output",
      "text": "Output must be valid for the destination format; if a destination schema is set, output is validated against it before sending."
    },
    {
      "id": "versions",
      "text": "Every transform version is kept, and the route detail shows which version produced which output."
    }
  ]
};
