import type { ComponentType } from 'react';
import { LookupSourceKind } from '../../../domain';
import { HttpFields, PostgresFields, RedisFields, StaticFields } from './LookupKindFields';
import type { LookupFormStepProps } from './model/LookupSourceForm';

/** Settings editor of each source kind (registry). */
export const LookupKindFields: Record<LookupSourceKind, ComponentType<LookupFormStepProps>> = {
  [LookupSourceKind.Http]: HttpFields,
  [LookupSourceKind.Redis]: RedisFields,
  [LookupSourceKind.Postgres]: PostgresFields,
  [LookupSourceKind.Static]: StaticFields,
};
