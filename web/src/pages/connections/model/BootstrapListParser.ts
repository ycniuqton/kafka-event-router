/**
 * Bootstrap server chips (CN-45): splits pasted lists on commas, whitespace and newlines,
 * drops duplicates and flags entries that are not `host:port` (IPv6 hosts in brackets).
 */
export class BootstrapListParser {
  static readonly invalidHint = 'host:port expected';
  static readonly maxPort = 65535;
  private static readonly separators = /[,\s]+/;
  private static readonly hasSeparator = /[,\s]/;
  private static readonly hostPort = /^(\[[0-9a-fA-F:.]+\]|[a-zA-Z0-9._-]+):(\d{1,5})$/;

  static split(raw: string): string[] {
    return raw
      .split(BootstrapListParser.separators)
      .map((x) => x.trim())
      .filter(Boolean);
  }

  static containsSeparator(raw: string): boolean {
    return BootstrapListParser.hasSeparator.test(raw);
  }

  /** Appends the parsed entries of `raw` that are not in `current` yet. */
  static add(current: string[], raw: string): string[] {
    const next = [...current];
    BootstrapListParser.split(raw).forEach((s) => {
      if (!next.includes(s)) next.push(s);
    });
    return next;
  }

  static isValid(server: string): boolean {
    const match = BootstrapListParser.hostPort.exec(server);
    if (!match) return false;
    const port = Number(match[2]);
    return port > 0 && port <= BootstrapListParser.maxPort;
  }

  static invalid(servers: string[]): string[] {
    return servers.filter((s) => !BootstrapListParser.isValid(s));
  }
}
