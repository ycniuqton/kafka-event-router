import { useEffect, useSyncExternalStore } from 'react';
import { AccessPolicy, type Access, type Permission } from '../domain';
import { accessStore } from './AccessStore';

/** The caller's effective access (null until loaded). */
export function useAccess(): Access | null {
  useEffect(() => accessStore.ensureLoaded(), []);
  return useSyncExternalStore(accessStore.subscribe, () => accessStore.getState().access);
}

/** Whether the caller may use `permission` (optionally on one team); false until access is loaded. */
export function useCan(permission: Permission, teamId?: string | null): boolean {
  return AccessPolicy.can(useAccess(), permission, teamId);
}
