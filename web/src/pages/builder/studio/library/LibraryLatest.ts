import { createContext, useContext, useEffect, useState } from 'react';
import { services } from '../../../../api';
import { LibraryKind, LibraryUpdates, type TransformStep } from '../../../../domain';

/** Latest versions of the library objects the steps reference (empty until loaded or on failure). */
export type LibraryLatest = { transforms: Record<string, number>; tables: Record<string, number> };

export const LibraryLatestContext = createContext<LibraryLatest>({ transforms: {}, tables: {} });

/** Reads the latest versions of referenced transforms and tables (GET /transforms?names=, /lookup-tables?names=). */
export function useLibraryLatestLoader(steps: TransformStep[]): LibraryLatest {
  const [latest, setLatest] = useState<LibraryLatest>({ transforms: {}, tables: {} });
  const names = LibraryUpdates.names(steps);
  const key = JSON.stringify(names);
  useEffect(() => {
    const wanted = JSON.parse(key) as ReturnType<typeof LibraryUpdates.names>;
    if (!wanted.transforms.length && !wanted.tables.length) return;
    let live = true;
    Promise.all([
      services.library.latestVersions(LibraryKind.Transform, wanted.transforms),
      services.library.latestVersions(LibraryKind.LookupTable, wanted.tables),
    ]).then(
      ([transforms, tables]) => live && setLatest({ transforms, tables }),
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [key]);
  return latest;
}

/** Latest versions provided by the steps pane. */
export function useLibraryLatest(): LibraryLatest {
  return useContext(LibraryLatestContext);
}
