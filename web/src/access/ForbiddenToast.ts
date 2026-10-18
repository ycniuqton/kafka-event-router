import { ApiError, ApiErrorListener, ProblemKind } from '../api/http/core';

/** Fallback toast text when a 403 problem carries no detail. */
export const ForbiddenText = { fallback: "You don't have permission to do this." } as const;

/**
 * 403 on any call → toast with the server's problem detail, then reload effective access
 * (the caller's role may have changed since the UI last gated its actions).
 */
export class ForbiddenToast extends ApiErrorListener {
  private readonly toast: (message: string) => void;
  private readonly onForbidden: () => void;

  constructor(toast: (message: string) => void, onForbidden: () => void = () => undefined) {
    super();
    this.toast = toast;
    this.onForbidden = onForbidden;
  }

  handle(error: ApiError): void {
    if (!error.is(ProblemKind.Forbidden)) return;
    this.toast(error.detail || ForbiddenText.fallback);
    this.onForbidden();
  }
}
