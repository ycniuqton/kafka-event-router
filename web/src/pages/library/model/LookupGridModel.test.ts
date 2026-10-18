import { describe, expect, it } from 'vitest';
import { LookupCsv } from '../../../domain';
import { LookupGridModel } from './LookupGridModel';

describe('LookupGridModel', () => {
  it('pastes CSV: replaces existing keys, appends new ones, keeps value types', () => {
    const grid = LookupGridModel.fromRows([{ key: 'DE', value: 'EU' }, { key: 'VN', value: { r: 'APAC' } }]);
    const { grid: next, issues } = grid.paste('key,value\nDE,EMEA\nUS,"AMER, north"\n');
    expect(issues).toEqual([]);
    expect(next.toRows()).toEqual([
      { key: 'DE', value: 'EMEA' },
      { key: 'VN', value: { r: 'APAC' } },
      { key: 'US', value: 'AMER, north' },
    ]);
  });

  it('reports pasted lines without a header or with duplicate keys and does not apply them', () => {
    expect(LookupGridModel.fromRows([]).paste('DE,EMEA').issues[0].line).toBe(1);
    expect(LookupGridModel.fromRows([]).paste('key,value\nA,1\nA,2\n,3').issues.map((i) => i.line)).toEqual([3, 4]);
  });

  it('lists empty and duplicate keys of edited rows and parses JSON-looking values', () => {
    const grid = LookupGridModel.fromRows([]).add().set(0, { key: 'a', value: '42' }).add().set(1, { key: 'a', value: 'x' }).add();
    expect(grid.issues().map((i) => i.line)).toEqual([2, 3]);
    expect(grid.remove(1).remove(1).toRows()).toEqual([{ key: 'a', value: 42 }]);
  });

  it('round-trips rows through CSV text', () => {
    const rows = [{ key: 'a', value: 'x, "y"' }, { key: 'b', value: { n: 1 } }];
    const back = LookupCsv.parse(LookupCsv.format(rows)).rows;
    expect(back[0]).toEqual(rows[0]);
    expect(back[1]).toEqual({ key: 'b', value: '{"n":1}' });
  });
});
