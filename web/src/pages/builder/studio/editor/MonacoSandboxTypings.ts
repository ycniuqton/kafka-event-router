import type * as Monaco from 'monaco-editor';

/** Installs the sandbox's `event`/`ctx` declarations (served by runtime-info) as a Monaco extra lib. */
export class MonacoSandboxTypings {
  static readonly uri = 'file:///event-router/sandbox-ctx.d.ts';
  private static installed: { typings: string; handle: Monaco.IDisposable } | null = null;

  static apply(monaco: typeof Monaco, typings: string | undefined): void {
    if (!typings || MonacoSandboxTypings.installed?.typings === typings) return;
    MonacoSandboxTypings.installed?.handle.dispose();
    const handle = monaco.typescript.javascriptDefaults.addExtraLib(typings, MonacoSandboxTypings.uri);
    MonacoSandboxTypings.installed = { typings, handle };
  }
}
