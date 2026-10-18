/** Available backends for the service registry. */
export const DataSource = { Mock: 'mock', Http: 'http' } as const;
export type DataSource = (typeof DataSource)[keyof typeof DataSource];

/** Runtime API configuration read from Vite env variables. */
export class ApiConfig {
  static readonly defaultBaseUrl = '/api/v1';

  static dataSource(): string {
    return import.meta.env.VITE_DATA_SOURCE ?? DataSource.Mock;
  }

  static baseUrl(): string {
    return import.meta.env.VITE_API_BASE_URL ?? ApiConfig.defaultBaseUrl;
  }
}

/** Simulated latencies of the mock backend. */
export const MockLatency = { requestMs: 120, connectionTestStepMs: 650 } as const;
