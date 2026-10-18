#!/usr/bin/env node
/**
 * Writes the last run (report/results.json, or the file in ACC_RESULTS) into TEST-CASES.md: the status column of the
 * run's target (Local / Staging), Automated by, the per-target summary and known failures (scripts/known-failures.json).
 */
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const Root = join(dirname(fileURLToPath(import.meta.url)), '..');

/** File locations and markers of the catalogue. */
const Files = {
  catalogue: join(Root, 'TEST-CASES.md'),
  results: process.env.ACC_RESULTS ?? join(Root, 'report', 'results.json'),
  reasons: join(Root, 'scripts', 'known-failures.json'),
};
const Markers = { summary: 'run-summary', failures: 'known-failures' };
const Status = { pass: 'pass', fail: 'fail', skip: 'skip', manual: 'manual', notRun: 'not run' };
const CaseId = /TC-[A-Z]+-\d+/g;

/** Targets and the catalogue column each one owns (cell index in a case row). */
const Targets = {
  local: { header: 'Local', cell: 8 },
  staging: { header: 'Staging', cell: 9 },
};
const Columns = { automatedBy: 7, legacyStatusHeader: 'Status', rowPrefix: '| TC-', headerPrefix: '| ID | Pri |' };

/** Outcomes of the last Playwright run, keyed by case id. */
class RunResults {
  constructor(json) {
    this.json = json;
    this.byCase = new Map();
    for (const suite of json.suites ?? []) this.walk(suite);
  }

  static load() {
    if (!existsSync(Files.results)) throw new Error(`no ${Files.results}: run the suite first`);
    return new RunResults(JSON.parse(readFileSync(Files.results, 'utf8')));
  }

  get target() {
    return this.json.config?.metadata?.target ?? 'local';
  }

  get startedAt() {
    return (this.json.stats?.startTime ?? new Date().toISOString()).slice(0, 19).replace('T', ' ') + ' UTC';
  }

  walk(suite) {
    for (const spec of suite.specs ?? []) this.record(spec);
    for (const child of suite.suites ?? []) this.walk(child);
  }

  record(spec) {
    const test = spec.tests?.[0];
    const result = test?.results?.at(-1);
    const status = RunResults.status(test?.status, result?.status);
    const ids = [...new Set(spec.title.match(CaseId) ?? [])];
    const where = `${test?.projectName ?? ''}/${basename(spec.file)} › ${ids[0] ?? spec.title}`;
    const error = (result?.error?.message ?? '').split('\n').find((l) => l.trim()) ?? '';
    for (const id of ids) this.merge(id, { status, where, error: RunResults.clean(error) });
  }

  merge(id, entry) {
    const prev = this.byCase.get(id);
    if (!prev || entry.status === Status.fail) this.byCase.set(id, entry);
  }

  static status(testStatus, resultStatus) {
    if (testStatus === 'skipped' || resultStatus === 'skipped') return Status.skip;
    return testStatus === 'expected' || testStatus === 'flaky' ? Status.pass : Status.fail;
  }

  static clean(text) {
    return text.replace(/\u001b\[[0-9;]*m/g, '').replace(/\|/g, '/').slice(0, 220);
  }
}

/** One case row of the catalogue, split into cells. */
class CaseRow {
  constructor(line) {
    this.cells = line.replace(/\s*\|\s*$/, '').split(' | ');
  }

  static matches(line) {
    return line.startsWith(Columns.rowPrefix) && line.split(' | ').length > Columns.automatedBy + 1;
  }

  get id() {
    return this.cells[0].slice(2).trim();
  }

  get manual() {
    return this.cells[Columns.automatedBy].trim().startsWith('manual');
  }

  get alias() {
    return this.cells[Columns.automatedBy].match(/\((TC-[A-Z]+-\d+)\)/)?.[1];
  }

  status(target) {
    return (this.cells[Targets[target].cell] ?? Status.notRun).trim();
  }

  /** Adds the staging cell to a row written before the per-target columns existed. */
  widen() {
    if (this.cells.length <= Targets.staging.cell) this.cells.push(this.manual ? Status.manual : Status.notRun);
  }

  /** Writes the run outcome of this case into the target's column. */
  apply(results, target) {
    const run = results.byCase.get(this.id) ?? (this.alias ? results.byCase.get(this.alias) : undefined);
    if (run && !this.cells[Columns.automatedBy].includes('(')) this.cells[Columns.automatedBy] = run.where;
    this.cells[Targets[target].cell] = run ? run.status : this.manual ? Status.manual : Status.notRun;
    return run;
  }

  toString() {
    return `${this.cells.join(' | ')} |`;
  }
}

/** The markdown catalogue: case rows, summary block and known-failures block. */
class Catalogue {
  constructor(text) {
    this.lines = text.split('\n');
  }

  static load() {
    return new Catalogue(readFileSync(Files.catalogue, 'utf8'));
  }

  /** Migrates tables with a single Status column to Local + Staging columns. */
  widen() {
    const legacy = `| ${Columns.legacyStatusHeader} |`;
    const wide = `| ${Targets.local.header} | ${Targets.staging.header} |`;
    this.lines = this.lines.map((line, i) => {
      if (line.startsWith(Columns.headerPrefix) && line.endsWith(legacy)) return line.slice(0, -legacy.length) + wide;
      if (line.startsWith('|---') && this.lines[i - 1]?.startsWith(Columns.headerPrefix) && line.split('|').length === 11) return `${line}---|`;
      if (!CaseRow.matches(line)) return line;
      const row = new CaseRow(line);
      row.widen();
      return row.toString();
    });
  }

  /** Updates the target column of every case row; returns the per-case rows with their run entries. */
  apply(results, target) {
    const rows = [];
    this.lines = this.lines.map((line) => {
      if (!CaseRow.matches(line)) return line;
      const row = new CaseRow(line);
      row.run = row.apply(results, target);
      rows.push(row);
      return row.toString();
    });
    return rows;
  }

  block(marker) {
    const start = this.lines.findIndex((l) => l.includes(`<!-- ${marker}:start -->`));
    const end = this.lines.findIndex((l) => l.includes(`<!-- ${marker}:end -->`));
    return { start, end, body: start < 0 || end < start ? [] : this.lines.slice(start + 1, end) };
  }

  replaceBlock(marker, body) {
    const { start, end } = this.block(marker);
    if (start < 0 || end < start) return;
    this.lines.splice(start + 1, end - start - 1, ...body);
  }

  save() {
    writeFileSync(Files.catalogue, this.lines.join('\n'));
  }
}

/** Builds the per-target summary and known-failures blocks, keeping the other target's previous entries. */
class Report {
  constructor(results, rows, reasons, previous) {
    this.results = results;
    this.rows = rows;
    this.reasons = reasons;
    this.previous = previous;
  }

  count(target, status) {
    return this.rows.filter((r) => r.status(target) === status).length;
  }

  lastRun(target) {
    if (target === this.results.target) return this.results.startedAt;
    const old = this.previous.summary.find((l) => l.startsWith(`| ${target} |`));
    return old?.split(' | ')[1] ?? 'never';
  }

  summaryRow(target) {
    const total = this.rows.length;
    const manual = this.rows.filter((r) => r.manual && r.status(target) === Status.manual).length;
    const counts = [Status.pass, Status.fail, Status.skip, Status.notRun].map((s) => this.count(target, s));
    return `| ${target} | ${this.lastRun(target)} | ${total} | ${total - manual} | ${manual} | ${counts.join(' | ')} |`;
  }

  summary() {
    return [
      `**Last run:** ${this.results.startedAt} · **target:** \`${this.results.target}\``,
      '',
      '| Target | Last run | Cases | Automated | Manual | Pass | Fail | Skip | Not run |',
      '|---|---|---|---|---|---|---|---|---|',
      ...Object.keys(Targets).map((t) => this.summaryRow(t)),
    ];
  }

  reason(id) {
    return this.reasons[`${this.results.target}:${id}`] ?? this.reasons[id] ?? 'not triaged yet';
  }

  failures() {
    const target = this.results.target;
    const kept = this.previous.failures.filter((l) => /^\| (local|staging) \| TC-/.test(l) && !l.startsWith(`| ${target} |`));
    const fresh = this.rows.filter((r) => r.status(target) === Status.fail).map((r) => `| ${target} | ${r.id} | ${this.reason(r.id)} | ${r.run.error} |`);
    if (kept.length + fresh.length === 0) return ['_None: every automated case passed on the last run of each target._'];
    return ['| Target | ID | Reason | Error of the last run |', '|---|---|---|---|', ...kept, ...fresh];
  }
}

const results = RunResults.load();
if (!Targets[results.target]) throw new Error(`unknown target ${results.target}`);
const catalogue = Catalogue.load();
catalogue.widen();
const previous = { summary: catalogue.block(Markers.summary).body, failures: catalogue.block(Markers.failures).body };
const rows = catalogue.apply(results, results.target);
const reasons = existsSync(Files.reasons) ? JSON.parse(readFileSync(Files.reasons, 'utf8')) : {};
const report = new Report(results, rows, reasons, previous);
catalogue.replaceBlock(Markers.summary, report.summary());
catalogue.replaceBlock(Markers.failures, report.failures());
catalogue.save();
console.log(report.summary().join('\n'));
