import type { ApiError } from './ApiError';
import type { HttpRequest } from './HttpRequest';

/** Base for app-wide reactions to failed calls (login redirect, conflict/network toasts). */
export abstract class ApiErrorListener {
  abstract handle(error: ApiError, request: HttpRequest): void;
}

/** Fans each failed request out to the registered listeners; requests marked `silent` skip it. */
export class ApiErrorBus {
  private readonly listeners = new Set<ApiErrorListener>();

  /** Registers a listener; returns the unsubscribe function. */
  add(listener: ApiErrorListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  publish(error: ApiError, request: HttpRequest): void {
    if (request.silent) return;
    this.listeners.forEach((listener) => listener.handle(error, request));
  }
}
