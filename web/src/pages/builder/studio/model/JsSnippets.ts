/** A reusable code pattern inserted into a JavaScript step. */
export type JsSnippet = { id: string; label: string; code: string };

/** Snippets menu of the JS editor (spec §8.5). */
export class JsSnippetCatalog {
  static readonly all: JsSnippet[] = [
    {
      id: 'drop-missing',
      label: 'Drop if field missing',
      code: `  if (event.value.items === undefined) return null;\n`,
    },
    {
      id: 'split-array',
      label: 'Split array into events',
      code: `  return event.value.items.map((item) => ({ ...event, value: { ...event.value, item } }));\n`,
    },
    {
      id: 'camel-case',
      label: 'Rename all keys to camelCase',
      code:
        `  const camel = (k) => k.replace(/_(\\w)/g, (_, c) => c.toUpperCase());\n` +
        `  event.value = Object.fromEntries(Object.entries(event.value).map(([k, v]) => [camel(k), v]));\n`,
    },
    {
      id: 'default-values',
      label: 'Default values for missing fields',
      code: `  event.value = { currency: 'EUR', status: 'new', ...event.value };\n`,
    },
    {
      id: 'header-to-payload',
      label: 'Copy a header into the payload',
      code: `  event.value.traceId = (event.headers || {}).traceId;\n`,
    },
    {
      id: 'mask-email',
      label: 'Mask email with ctx',
      code: `  event.value.email = ctx.mask.email(event.value.email);\n`,
    },
    {
      id: 'log-value',
      label: 'Log a value',
      code: `  ctx.log('amount', event.value.amount);\n`,
    },
  ];
}
