import { DocumentFormat, type ImportReport, type RouteExport } from '../../../domain';
import { HttpHeader } from '../core/HttpConstants';
import type { ImportReportDto } from './RouteDtos';

/** Reads export downloads (attachment name, media type, text) and import reports of `/routes`. */
export class RoutePortabilityMapper {
  static readonly dispositionHeader = 'Content-Disposition';
  static readonly fallbackStem = 'routes-export';
  private static readonly fileNamePattern = /filename="?([^";]+)"?/;
  private static readonly mediaTypes: Record<DocumentFormat, string> = {
    [DocumentFormat.Yaml]: 'application/yaml',
    [DocumentFormat.Json]: 'application/json',
  };

  static async exportFile(response: Response, format: DocumentFormat): Promise<RouteExport> {
    const disposition = response.headers.get(RoutePortabilityMapper.dispositionHeader) ?? '';
    const name = RoutePortabilityMapper.fileNamePattern.exec(disposition)?.[1] ?? `${RoutePortabilityMapper.fallbackStem}.${format}`;
    const mediaType = response.headers.get(HttpHeader.ContentType) ?? RoutePortabilityMapper.mediaTypes[format];
    return { fileName: name, mediaType, content: await response.text() };
  }

  static report(dto: ImportReportDto): ImportReport {
    return {
      dryRun: dto.dryRun,
      items: dto.results.map((r) => ({
        index: r.index,
        name: r.name,
        routeId: r.routeId ?? null,
        action: r.action,
        issues: r.issues ?? [],
      })),
    };
  }
}
