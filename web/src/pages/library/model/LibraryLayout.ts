/** Grid layout of the design's library rows. */
export const LibraryLayout = {
  columns: 'minmax(0,1fr) 50px minmax(0,1.6fr) 90px 100px',
  lookupColumns: 'minmax(0,1fr) 50px minmax(0,1.6fr) 90px 90px 100px',
  rowColumns: 'minmax(0,1fr) minmax(0,2fr) 32px',
} as const;

/** Tabs of the library page. */
export const LibraryTab = { Transforms: 'transforms', LookupTables: 'lookup-tables' } as const;
export type LibraryTab = (typeof LibraryTab)[keyof typeof LibraryTab];

/** Query keys of the library page (tab, search text and the open object). */
export const LibraryParam = { Tab: 'tab', Search: 'q', Item: 'item' } as const;

/** Tabs of a detail drawer. */
export const DetailTab = { Versions: 'versions', Usages: 'usages', TestCases: 'tests', Rows: 'rows' } as const;
export type DetailTab = (typeof DetailTab)[keyof typeof DetailTab];

/** Texts of the library page. */
export class LibraryText {
  static readonly title = 'Transform library';
  static readonly subtitle = 'Versioned transforms and lookup tables any route can use. Routes never update automatically.';
  static readonly search = 'Search by name';
  static readonly tabs = [
    { id: LibraryTab.Transforms, label: 'Transforms' },
    { id: LibraryTab.LookupTables, label: 'Lookup tables' },
  ];
  static readonly newTable = 'New lookup table';
  static readonly loadError = 'The library could not be loaded.';
  static readonly noTransforms = 'No saved transforms yet.';
  static readonly noTransformsHint = 'Save steps from the Transform Studio to reuse them in other routes.';
  static readonly noTables = 'No lookup tables yet.';
  static readonly noTablesHint = 'Lookup tables map a key to a value (e.g. country → region) for lookup steps.';
  static readonly noMatch = 'Nothing matches this search.';
  static readonly description = 'Description';
  static readonly saveDescription = 'Save description';
  static readonly archive = 'Archive';
  static readonly archived = (name: string) => `Archived ${name}`;
  static readonly descriptionSaved = 'Description saved';
  static readonly versions = 'Versions';
  static readonly usages = 'Usages';
  static readonly testCases = 'Test cases';
  static readonly rows = 'Rows';
  static readonly noUsages = 'No route uses this yet.';
  static readonly draft = 'draft';
  static readonly active = 'deployed';
  static readonly rowsCount = (n: number) => `${n.toLocaleString()} rows`;
  static readonly transformTabs = [
    { id: DetailTab.Versions, label: LibraryText.versions },
    { id: DetailTab.Usages, label: LibraryText.usages },
    { id: DetailTab.TestCases, label: LibraryText.testCases },
  ];
  static readonly tableTabs = [
    { id: DetailTab.Rows, label: LibraryText.rows },
    { id: DetailTab.Versions, label: LibraryText.versions },
    { id: DetailTab.Usages, label: LibraryText.usages },
  ];
}
