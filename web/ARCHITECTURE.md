# Event Router web — architecture brief

Read this before building a screen. Sources: the design prototype (`/home/dev/Event Router.dc.html`), the
product spec and the system design (`/api/v1`, §5–6). Vite 8 + React 19 + TS, react-router-dom 6,
CSS Modules. Commands: `npm run dev`, `npm run build` (must stay at zero TS errors), `npx oxlint` (must stay clean).

## Folder layout

```
src/
  main.tsx              entry: global CSS, store init, <App/>
  styles/               tokens.css (dark/light vars, verbatim from design + data-tone hooks), global.css
  domain/               types, `as const` enums, presentation maps, business rules (no React)
    pipeline/           filter + transform engine: FieldPath, FilterEvaluator, FilterExpressionBuilder,
                        TransformRunner (StepHandlerRegistry), ValueMasker/ValueHasher, JsSandbox, JsonFlattener
  mocks/                static fake data ported 1:1 from the design script (ONLY api/mock may import it)
  api/                  contracts (abstract service classes), mock/ implementations, registry (`services`)
  store/                app store (useSyncExternalStore), action classes, settings, keyboard shortcuts
  components/ui/        shared primitives (each with its own .module.css) + Format
  app/                  shell (TopBar, AlertsMenu, SideNav, NewRouteModal), router, paths, nav config
  pages/<screen>/       one folder per screen: <Name>Page.tsx (default export) + <Name>Page.module.css
```

Screens: `dashboard`, `routes`, `route-detail`, `builder` (Transform Studio = `step=transform`),
`connections`, `library`, `dead-letters`, `audit`, `settings`, `not-found`. Put screen-private components
in `pages/<screen>/components/`. Promote something to `components/ui` only if two screens need it.

## Routes (`app/router.tsx`, URLs built only with `AppPaths` from `app/paths.ts`)

| URL | Page | Notes |
| --- | --- | --- |
| `/` | DashboardPage | |
| `/routes?q=&status=` | RoutesPage | `q` is driven by the top-bar search |
| `/routes/:id?tab=overview\|tail\|dead\|history` | RouteDetailPage | `RouteDetailTab` |
| `/routes/new?template=filter\|partner\|merge\|blank` | BuilderPage | fullscreen (no left nav) |
| `/routes/:id/edit?step=source\|filter\|transform\|destination\|review&sample=<id>` | BuilderPage | `sample` preselects a studio event |
| `/connections?test=<id>` | ConnectionsPage | `test`: open the test drawer for that id and start the test if none ran (`useConnectionTest`) |
| `/library`, `/dead-letters`, `/audit`, `/settings` | … | |

Pages are lazy-loaded; route `handle` (`RouteHandle`) sets the active nav entry and `fullscreen`.
Read query params with `useSearchParams()` and the `QueryParam` keys; navigate with `AppPaths.*()`.

## Data access — `services` (the Manager)

```ts
import { services } from '../../api';
const routes = await services.routes.list();
```

`services` is chosen by `VITE_DATA_SOURCE` (default `mock`) in `ServiceRegistry`. Every service is an abstract
class in `api/contracts.ts`; mocks extend them (`api/mock/*`), sharing one in-memory `MockDatabase` with ~120 ms
latency. A REST implementation is added later by `ServiceRegistry.register(DataSource.Http, factory)` — screens
never change. **Screens import only `services` (or the store), never `mocks/` or `api/mock`.**

| Service | Methods |
| --- | --- |
| `routes` | `list, get, pause, resume, duplicate, archive, openDraft(routeId \| null, templateId?), saveDraft, deploy(draft, note) → {route, version, activity}, listVersions, rollback(routeId, version)` |
| `connections` | `list, create(ConnectionDraft), remove, test(id, onUpdate) → ConnectionTestRun, listTopics, createTopic` |
| `deadLetters` | `page(filter, cursor?)` (server scan, newest first), `summary(routeId?)`, `detail`, `retry(ids)`, `retryMatching(routeId, filter)`, `discard(ids)`, `exportJson(routeId, selection)`, `openInStudio` |
| `samples` | `list(conn, topic)` (24 order events), `peek(conn, topic, n)`, `fields()` → `{path, numeric}[]` |
| `activity` / `audit` / `library` | `list()` |
| `alerts` | `list()` (top bar), `attention()` (dashboard "Needs attention") |
| `metrics` | `dashboard()` → `{eventsLastHour, sparkSeed}` |
| `members` | `list()`, `current()` |
| `sandbox` | `runtimeInfo()` → limits, ctx helper docs, Monaco typings, safety rules (`/sandbox/runtime-info`, read once per page load) |
| `apiTokens` | `list()`, `create(draft) → {token, secret}` (secret shown once), `revoke(id)` (`/auth/tokens`, Settings → API tokens) |
| `clock` | `now()` — mock clock starts at 2026-10-13T09:44Z |

## Shared state — `store/`

Collections touched by several screens live in one store; everything else is page-local.

```ts
import { actions, useRoutes, useRoute, useVersions, useServiceQuery } from '../../store';
const routes = useRoutes();                       // Route[]
await actions.routes.togglePause(id);             // updates store + shows the design's toast
const { data: audit } = useServiceQuery(() => services.audit.list(), []);  // page-local data
```

Hooks: `useAppReady, useRoutes, useRoute(id), useConnections, useConnectionTest(id), useDeadLetterSummary(), useDeadLetterCount(routeId?),
useActivity, useVersions(routeId)` (fetches on first use), `useAlerts, useCurrentUser, useToast,
useNewRouteChooserOpen, useSettings, useNow()` (clock, 30 s refresh), `useServiceQuery(load, deps)`,
`useAppSelector(selector)` — selectors must return a stored reference; derive filtered arrays with `useMemo`.

Actions (all async, all update the store and flash the design's toast text):
- `actions.routes`: `togglePause, bulkPause(ids), bulkResume(ids), duplicate, archive, saveDraft, deploy(draft, note) → Route, loadVersions, rollback`
- `actions.connections`: `test(id)` (live progress in `useConnectionTest`), `create(draft)` (auto-tests), `remove(id)` (→ `false` if used by routes; `usedBy(id)`), `createTopic, clearTest`
- `actions.deadLetters`: `page, refreshSummary, retry(ids), retryMatching, discard(ids), detail, exportJson, openInStudio` (the store keeps only the badge summary; lists are page-local in `DeadLetterBrowser`)
- `actions.toast.show(message)`; `actions.openNewRouteChooser()` / `closeNewRouteChooser()`
- `settingsStore.setTheme / toggleTheme / setShowPhases` (persisted in localStorage, safe)

Alert / attention rows carry an `ActionIntent`; run it with `useIntentRunner()` from `app/useIntentRunner.ts`.

**Keyboard:** `useShortcut({ key, mod?, allowInInputs?, handler }, enabled)`. The shell owns `N` (new route) and
`/` (search); Modal, Drawer and the alerts menu own `Escape` while open (newest registration wins). The builder
registers `Ctrl/⌘+S` (save draft) and `Ctrl/⌘+Enter` (test run) with `mod: true` (these fire inside inputs).

## Domain (`domain/`)

Enums (`as const` + same-named type; `erasableSyntaxOnly` forbids TS `enum`): `RouteStatus, TransformMode,
ConnectionStatus, EnvTag, SecurityMode, TestStepState, FilterOp, FilterCombinator, FilterMode, TransformStepKind,
MaskMode, BuilderStep, StartPosition, RouteTemplateId, DeadLetterKind, DiffMark, ActivityVerb, Role, Theme,
Phase, RouteDetailTab, IntentType, Tone`.
Presentation maps: `RouteStatusMeta, ConnectionStatusMeta, EnvMeta, TransformModeMeta, DiffMarkTone,
StepKindLabel, MaskModeLabel, StepMenu`. Rules: `RouteRules, LagHealth` (thresholds 2k/20k per route,
10k/40k total), `DraftSummary, VersionDiff, StepFactory, RouteDraftFactory, TemplateCatalog`.
Option lists: `StringFilterOps, NumberFilterOps, DestinationOptions, ConnectionTestChecks, DefaultJsStep`.

Status colours (spec §12) are tones: running `ok` green, warn/lag `warn` amber, error `err` red, paused/filtered
`off` grey, draft `info` blue, prod env `prod`. Never hard-code a colour — pass a `Tone`.

Pipeline (builder, studio, tail, dead letters): `FilterEvaluator.evaluate(event, filter) → {pass, rowResults, error}`,
`FilterExpressionBuilder.build(filter)`, `TransformRunner.run(event, steps, uptoIndex?) → {event, dropped, error,
stepIndex, logs}`, `ValueMasker.mask(v, mode)`, `ValueHasher.hash(v)`, `JsonFlattener.flatten(obj) / leafMap(obj)`,
`FieldPath.get/set/remove/short`. Builder UI-only state from `configFor` (samplesOpen, sView, expanded, why,
studioSel, diff, atStep, selLine, addMenu, aiText, proposals…) belongs in the builder page, not in `RouteDraft`.

Design → domain field names (values are identical; relative-time labels became ISO timestamps that
`Format.relativeTime` turns back into "12 min ago", "yesterday", "2 w ago"):

| Design | Domain |
| --- | --- |
| route `src, topics, dc, dt, fsum, tx, eps, match, lag, changed, ver` | `source.connectionId, source.topics, destination.connectionId, destination.topic, filterSummary, transformMode, metrics.{eventsPerSec, matchPct, lag}, updatedAt, version` |
| conn `servers, sec, checked, topics[[n,p,rate]]` | `bootstrapServers, security, checkedAt, topics[{name, partitions, rate}]` |
| filter `comb, rows[{f,op,v}], mode, expr` | `combinator, rows[{field, op, value}], mode, expression` |
| step `type, on, a, b, mode` | `kind, enabled, field, argument, maskMode` |
| DL `time, type, msg, part, route, sid` | `failedAt, kind, message, partition, routeId, sampleId` (+ `stack`) |
| version `v, who, when, diff[[m,t]]` | `version, author, createdAt, diff[{mark, text}]` |
| activity `[who, what, route, extra, when]` | `{actor, verb, routeName, detail, at}` |
| `src.conn/start/multi`, `dest.conn/key/part/onErr` | `source.connectionId/start/multi`, `destination.connectionId/key/partitioning/onError` |

## Shared components (`components/ui`, import from `'../../components/ui'`)

| Component | Props |
| --- | --- |
| `Button` | `variant: primary\|secondary\|ghost\|danger`, `size: sm(26)\|md(30)\|lg(32)`, `square`, `block`, native button props |
| `StatusBadge` / `ToneBadge` | `{status: RouteStatus, dot?}` / `{tone, children, dot?}` |
| `EnvTag` | `{env}` |
| `HealthDot` | `{tone, label?, pulse?, large?}` (use with `ConnectionStatusMeta[s]`) |
| `Sparkline` | `{seed, flat?, tone?, width=54, height=18, wide?}`; `SparkSeries.heights(seed, flat)` (design `spark`) |
| `Card` (= `Panel`), `CardHeader` | `{padding: none\|sm\|md, overflow: clip\|scroll\|visible, interactive}`; `{title, meta?, actions?}` |
| `Page`, `PageHeader` | `{width = PageWidth.Wide\|Audit\|Library\|Narrow, relaxed?}`; `{title, subtitle?, actions?, back?, children?}` |
| `Tabs` | `{items: {id,label}[], value, onChange}` |
| `SegmentedControl` | `{options: T[] \| {value,label}[], value, onChange, size: xs\|sm\|md, mono?}` |
| `Chip` | `{active?, count?, onClick, children}` |
| `Modal` | `{open, onClose, title?, subtitle?, width = ModalWidth.Sm(460)\|Md\|Lg(760), footer?, hideClose?}` |
| `Drawer` | `{open, onClose, title, width = 460, footer?}` (right side panel) |
| `DataTable`, `TableHead`, `TableRow`, `TableCell` | `{columns: grid-template string, minWidth?}`; row `{onClick?, selected?}` |
| `EmptyState` | `{title, description?, action?, compact?}` |
| `Mono`, `Kbd`, `Tooltip` | `{size: xs\|sm\|md, tone?, medium?, truncate?}`; key hint; `{label, underline?}` (native title) |
| `JsonTree` | `{value, collapsible = true, initiallyCollapsed?, onLineClick?(line), decorate?(line) → {highlight: added\|changed\|selectedLine, mark, title, trailing}}` |
| `TextInput`, `Select`, `TextArea`, `Field` | `controlSize: sm\|md\|lg, mono, block, sunken`; `Field {label, hint?, error?}` |
| `ToastHost` | mounted by the shell — call `actions.toast.show()` |
| `Format` | `number` (1.84M / 48.2k / 2,310), `rate`, `percent`, `relativeTime(iso, now)`, `clockTime`, `monthDayTime`, `versionDate`, `shortDate` (UTC) |

## Styling conventions

- One `.module.css` per component/page; tokens only (`var(--p1)`, `var(--tx2)`, `var(--acc)`, …) plus the additive
  tokens in `tokens.css` (`--font-mono`, `--radius-*`, `--shadow-*`, `--z-*`, `--topbar-h`).
- Tone colours: set `data-tone={tone}` on an element and use `var(--tone)` in its CSS.
- **No inline styles** except dynamic values passed as CSS custom properties (`style={{'--h': '42%'}}`).
- Fonts: Geist 13px/1.45 body; add `.mono` class or `<Mono>` for topics, field paths, values, offsets, code.
- Keep the design's sizes (row padding 10px 14px, card radius 10px, page padding 24/28/40, h1 20px/600).
- Theme is `data-theme` on `<html>`; both themes must work — never use raw hex colours.

## Rules for screen agents

1. Replace only your `pages/<screen>/` placeholder (keep the default export and file name). Don't edit
   `app/`, `store/`, `api/`, `domain/`, `components/ui` without coordinating — extend in your page folder first.
2. Data via `services` / store hooks only; mutations shared with other screens go through `actions.*`.
3. Coding rules (`.devlens/rules.md`): functions ≤ 50 lines, ≤ 3 nesting levels, if/loop bodies ≤ 15 lines;
   comments only at the top of a module/function/class (≤ 3 lines, intent not implementation);
   no magic strings/numbers — use domain `as const` objects or add a named constant to a domain class;
   prefer classes for non-trivial logic (view-model builders, calculators); no free helper functions.
4. Copy text, numbers and toasts from the design verbatim; derive view values (e.g. lag colour via `LagHealth`,
   `RouteStatusMeta`) instead of re-implementing the prototype's inline ternaries.
5. Use `AppPaths` for every link and `useShortcut` for keys. `npm run build` and `npx oxlint` must pass.
