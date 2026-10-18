import { describe, expect, it } from 'vitest';
import { LibraryUpdates, LookupMode, StepFactory, TransformStepKind } from '../../../../domain';
import { LibraryPin } from './LibraryPin';

describe('Update available', () => {
  it('reports only strictly newer latest versions', () => {
    expect(LibraryUpdates.available(2, 3)).toBe(3);
    expect(LibraryUpdates.available(3, 3)).toBeNull();
    expect(LibraryUpdates.available(4, 3)).toBeNull();
    expect(LibraryUpdates.available(1, undefined)).toBeNull();
  });

  it('pins library steps and named lookup tables, not inline lookups', () => {
    const lib = StepFactory.create(TransformStepKind.Library, { libraryRef: 'mask-pii', libraryVersion: 2 });
    const table = StepFactory.create(TransformStepKind.Lookup, { field: 'c', lookupMode: LookupMode.Table, lookupRef: 'regions', lookupVersion: 1 });
    const inline = StepFactory.create(TransformStepKind.Lookup, { field: 'c', lookupRows: [{ key: 'a', value: 1 }] });
    expect(LibraryPin.of(lib)).toMatchObject({ name: 'mask-pii', version: 2 });
    expect(LibraryPin.of(lib)?.patch(3)).toEqual({ libraryVersion: 3 });
    expect(LibraryPin.of(table)?.patch(4)).toEqual({ lookupVersion: 4 });
    expect(LibraryPin.of(inline)).toBeNull();
    expect(LibraryUpdates.names([lib, table, inline, lib])).toEqual({ transforms: ['mask-pii'], tables: ['regions'] });
  });
});
