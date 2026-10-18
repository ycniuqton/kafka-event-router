import { services } from '../../../../api';
import type { RuntimeInfo } from '../../../../domain';
import { useServiceQuery } from '../../../../store';

/** Sandbox runtime info (limits, ctx docs, typings, safety rules) for the JS step editor. */
export function useSandboxInfo(): { info: RuntimeInfo | undefined; loading: boolean; error: Error | undefined; reload(): void } {
  const { data, loading, error, reload } = useServiceQuery(() => services.sandbox.runtimeInfo(), []);
  return { info: data, loading, error, reload };
}
