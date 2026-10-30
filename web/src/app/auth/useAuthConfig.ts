import type { AuthConfig } from '../../api/http/core';
import { useServiceQuery } from '../../store';
import { AuthConfigStore } from './AuthConfigStore';

/** The server's auth config (null while loading or when it failed). */
export function useAuthConfig(): AuthConfig | null {
  return useServiceQuery(() => AuthConfigStore.load(), []).data ?? null;
}
