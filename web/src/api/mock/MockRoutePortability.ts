import {
  DocumentFormat,
  ImportAction,
  ImportMode,
  IssueSeverity,
  RouteStatus,
  type ImportItem,
  type ImportOptions,
  type ImportReport,
  type Route,
  type RouteExport,
} from '../../domain';

/** Minimal route document of the mock export (connection names instead of ids, like the server). */
type MockDocument = {
  apiVersion: string;
  name: string;
  source: { connectionId: string; topics: string[] };
  branches: {
    id: string;
    destination: { connectionId: string; topic: string };
  }[];
};

/** Mock export / import: JSON documents (a YAML stream of JSON documents is valid YAML). */
export class MockRoutePortability {
  static readonly apiVersion = 'router/v1';
  static readonly branchId = 'main';
  static readonly separator = '---\n';
  static readonly bulkStem = 'routes-export';
  static readonly newRouteStatus = RouteStatus.Draft;
  private static readonly splitter = /^---\s*$/m;
  private static readonly mediaTypes: Record<DocumentFormat, string> = {
    [DocumentFormat.Yaml]: 'application/yaml',
    [DocumentFormat.Json]: 'application/json',
  };

  static export(routes: Route[], format: DocumentFormat): RouteExport {
    const docs = routes.map((r) => MockRoutePortability.document(r));
    const content =
      format === DocumentFormat.Json
        ? JSON.stringify(docs.length === 1 ? docs[0] : docs, null, 2)
        : docs.map((d) => `${MockRoutePortability.separator}${JSON.stringify(d, null, 2)}\n`).join('');
    const stem = routes.length === 1 ? routes[0].name : MockRoutePortability.bulkStem;
    return {
      fileName: `${stem}.${format}`,
      mediaType: MockRoutePortability.mediaTypes[format],
      content,
    };
  }

  /** Classifies each document; `create` adds a draft route for new names unless it is a dry run. */
  static import(content: string, options: ImportOptions, routes: Route[], create: (doc: MockDocument) => Route): ImportReport {
    const items = MockRoutePortability.parse(content).map((doc, index): ImportItem => {
      if (!doc?.name)
        return {
          index,
          name: '',
          routeId: null,
          action: ImportAction.Invalid,
          issues: [MockRoutePortability.issue('/name', 'Give the route a name.')],
        };
      const existing = routes.find((r) => r.name === doc.name);
      if (existing) {
        const action = options.mode === ImportMode.Upsert ? ImportAction.DraftUpdated : ImportAction.SkippedExists;
        return {
          index,
          name: doc.name,
          routeId: existing.id,
          action,
          issues: [],
        };
      }
      const routeId = options.dryRun ? null : create(doc).id;
      return {
        index,
        name: doc.name,
        routeId,
        action: ImportAction.Created,
        issues: [],
      };
    });
    return { dryRun: options.dryRun, items };
  }

  private static document(r: Route): MockDocument {
    return {
      apiVersion: MockRoutePortability.apiVersion,
      name: r.name,
      source: {
        connectionId: r.source.connectionName ?? r.source.connectionId,
        topics: r.source.topics,
      },
      branches: [
        {
          id: MockRoutePortability.branchId,
          destination: {
            connectionId: r.destination.connectionName ?? r.destination.connectionId,
            topic: r.destination.topic,
          },
        },
      ],
    };
  }

  private static parse(content: string): (MockDocument | null)[] {
    const trimmed = content.trim();
    if (trimmed.startsWith('[')) return MockRoutePortability.json(trimmed) as (MockDocument | null)[];
    return trimmed
      .split(MockRoutePortability.splitter)
      .map((part) => part.trim())
      .filter(Boolean)
      .map((part) => MockRoutePortability.json(part) as MockDocument | null);
  }

  private static json(text: string): unknown {
    try {
      return JSON.parse(text);
    } catch {
      return null;
    }
  }

  private static issue(path: string, message: string) {
    return {
      code: 'document_invalid',
      severity: IssueSeverity.Error,
      path,
      message,
    };
  }
}
