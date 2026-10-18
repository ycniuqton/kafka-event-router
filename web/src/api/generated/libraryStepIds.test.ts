/* Library step ids in traces follow the engine catalogue conventions (pipeline-engine PE-105). */
import { describe, expect, it } from 'vitest';
import { libraryCatalog, LibraryStepIds } from './index';

describe('library step ids', () => {
  it('exposes the library catalogue entry', () => {
    expect(libraryCatalog.pinnedKinds).toEqual(['library', 'lookup']);
    expect(libraryCatalog.lookupTableNames).toContain('<ref>@<version>');
  });

  it('parses a spliced step id', () => {
    expect(LibraryStepIds.parse('clean/orders-clean@2/mask')).toEqual({
      stepId: 'clean',
      ref: 'orders-clean',
      version: 2,
      childStepId: 'mask',
    });
  });

  it('parses nested library and if_else ids by their outermost pin', () => {
    expect(LibraryStepIds.parse('ie/thenSteps/lib/a@1/in/b@3/r')).toEqual({
      stepId: 'ie/thenSteps/lib',
      ref: 'a',
      version: 1,
      childStepId: 'in/b@3/r',
    });
  });

  it('ignores ids without a library pin', () => {
    expect(LibraryStepIds.parse('rename')).toBeNull();
    expect(LibraryStepIds.parse('ie/thenSteps/x')).toBeNull();
    expect(LibraryStepIds.parse('a@1')).toBeNull();
  });

  it('attributes trace steps to the route step', () => {
    expect(LibraryStepIds.belongsTo('clean/orders-clean@2/mask', 'clean')).toBe(true);
    expect(LibraryStepIds.belongsTo('clean', 'clean')).toBe(true);
    expect(LibraryStepIds.belongsTo('other/orders-clean@2/mask', 'clean')).toBe(false);
  });
});
