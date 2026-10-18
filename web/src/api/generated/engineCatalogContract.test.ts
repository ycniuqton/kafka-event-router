/* Guards the FE ↔ engine contract: every vocabulary the builder and Studio emit must exist in the generated engine catalogue. */
import { describe, expect, it } from 'vitest';
import { NumberFilterOps, StringFilterOps } from '../../domain/filter';
import { OperatorCatalog } from '../../domain/routeConfig/OperatorCatalog';
import { StepMapper } from '../../domain/routeConfig/StepMapper';
import { MaskMode, StepFactory, TransformStepKind } from '../../domain/transform';
import { engineCatalog } from './engineCatalog';
import { formulaFunctions } from './index';
import type { FieldType } from './engineCatalogTypes';

const FilterOpsByFieldType: [FieldType, typeof StringFilterOps][] = [
  ['string', StringFilterOps],
  ['number', NumberFilterOps],
];

describe('engine catalogue contract', () => {
  it.each(FilterOpsByFieldType)('builder operators for %s fields are engine operators for that type', (fieldType, ops) => {
    const allowed = engineCatalog.operatorsByFieldType[fieldType] ?? [];
    for (const op of ops) expect(allowed).toContain(OperatorCatalog.engine(op).op);
  });

  it('studio step kinds map to engine recipe kinds', () => {
    const kinds = engineCatalog.recipes.map((r) => r.kind);
    for (const kind of Object.values(TransformStepKind)) {
      expect(kinds).toContain(StepMapper.toStep(StepFactory.create(kind)).kind);
    }
  });

  it('studio mask modes are engine mask styles', () => {
    for (const mode of Object.values(MaskMode)) expect(engineCatalog.maskStyles).toContain(mode);
  });

  it('publishes the formula function catalogue for Studio autocomplete', () => {
    const names = formulaFunctions.map((f) => f.name);
    for (const name of ['concat', 'round', 'if', 'lookup', 'formatDate']) expect(names).toContain(name);
    for (const f of formulaFunctions) {
      expect(f.call.startsWith(`${f.name}(`)).toBe(true);
      expect(f.arity.min).toBeLessThanOrEqual(f.arity.max === -1 ? Infinity : f.arity.max);
    }
  });

  it('lists the P2 recipe kinds and groups', () => {
    const kinds = engineCatalog.recipes.map((r) => r.kind);
    for (const kind of ['flatten', 'redact_pattern', 'lookup', 'if_else', 'split_array', 'header_add', 'map'] as const) {
      expect(kinds).toContain(kind);
    }
    expect(engineCatalog.valueSources).toContain('formula');
    expect(engineCatalog.recipeGroups).toEqual(expect.arrayContaining(['structure', 'kafka']));
  });
});
