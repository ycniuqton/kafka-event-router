import type { DashboardStats } from '../domain';

/** Dashboard KPI values not derived from routes ("1.84M" events, spark seed 5). */
export const DASHBOARD_STATS: DashboardStats = { eventsLastHour: 1_840_000, sparkSeed: 5 };
