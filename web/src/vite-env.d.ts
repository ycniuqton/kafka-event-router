/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Which data source backs the service registry: "mock" (default) or "http". */
  readonly VITE_DATA_SOURCE?: string;
  /** Base URL of the control-plane REST API (default "/api/v1"). */
  readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
