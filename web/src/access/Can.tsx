import type { ReactNode } from 'react';
import type { Permission } from '../domain';
import { useCan } from './useCan';

export type CanProps = { permission: Permission; teamId?: string | null; fallback?: ReactNode; children: ReactNode };

/** Renders children only when the caller holds the permission (otherwise `fallback`). */
export function Can({ permission, teamId, fallback = null, children }: CanProps) {
  return <>{useCan(permission, teamId) ? children : fallback}</>;
}
