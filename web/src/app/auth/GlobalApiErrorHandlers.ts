import { ApiError, ApiErrorKind, ApiErrorListener, ApiErrorText, ProblemKind, type ApiErrorBus } from '../../api/http/core';
import { accessStore } from '../../access/AccessStore';
import { ForbiddenToast } from '../../access/ForbiddenToast';

/** 401 on any call mid-session → back to the login screen (the feature keeps its draft). */
export class UnauthenticatedRedirect extends ApiErrorListener {
  private readonly toLogin: () => void;

  constructor(toLogin: () => void) {
    super();
    this.toLogin = toLogin;
  }

  handle(error: ApiError): void {
    if (error.is(ProblemKind.Unauthenticated)) this.toLogin();
  }
}

/** Stale If-Match (412) → "someone else changed this" toast. */
export class ConflictToast extends ApiErrorListener {
  private readonly toast: (message: string) => void;

  constructor(toast: (message: string) => void) {
    super();
    this.toast = toast;
  }

  handle(error: ApiError): void {
    if (error.is(ProblemKind.PreconditionFailed)) this.toast(ApiErrorText.Conflict);
  }
}

/** No response at all → "can't reach the server" toast. */
export class NetworkToast extends ApiErrorListener {
  private readonly toast: (message: string) => void;

  constructor(toast: (message: string) => void) {
    super();
    this.toast = toast;
  }

  handle(error: ApiError): void {
    if (error.kind === ApiErrorKind.Network) this.toast(ApiErrorText.Network);
  }
}

export type GlobalHandlerTargets = { toLogin: () => void; toast: (message: string) => void };

/** Installs the app-wide API error reactions; returns the uninstall function. */
export class GlobalApiErrorHandlers {
  static install(bus: ApiErrorBus, targets: GlobalHandlerTargets): () => void {
    const removers = [
      bus.add(new UnauthenticatedRedirect(targets.toLogin)),
      bus.add(new ConflictToast(targets.toast)),
      bus.add(new NetworkToast(targets.toast)),
      bus.add(new ForbiddenToast(targets.toast, () => void accessStore.refresh())),
    ];
    return () => removers.forEach((remove) => remove());
  }
}
