import { useEffect, useState } from 'react';
import { services } from '../../../../api';
import type { LookupSource } from '../../../../domain';
import { LookupSourceCatalog } from '../model/LookupSourceCatalog';

/** Loads the lookup sources once into `LookupSourceCatalog` and re-renders the caller when they arrive. */
export function useLookupSourceCatalog(): LookupSource[] {
  const [sources, setSources] = useState<LookupSource[]>(() => LookupSourceCatalog.all());
  useEffect(() => {
    let live = true;
    services.lookupSources.list().then(
      (list) => {
        LookupSourceCatalog.load(list);
        if (live) setSources(list);
      },
      () => live && setSources([]),
    );
    return () => {
      live = false;
    };
  }, []);
  return sources;
}
