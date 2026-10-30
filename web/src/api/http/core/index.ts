/** Public surface of the REST core: client, errors, pagination, ETags, WebSocket base, service modules, session. */
export { HttpClient, type FetchFn, type HttpClientOptions } from './HttpClient';
export type { HttpRequest, HttpResponse, Query, QueryValue, RequestOptions } from './HttpRequest';
export { HttpMethod, HttpHeader, HttpStatus, MediaType, MutatingMethods } from './HttpConstants';
export { RawBody, UrlBuilder } from './HttpMessages';
export { ApiError, ApiErrorKind, ApiErrorText, type Problem, type FieldError } from './ApiError';
export { ApiErrorBus, ApiErrorListener } from './ApiErrorBus';
export { ProblemKind, ProblemCatalog } from './ProblemKind';
export { JsonPointer } from './JsonPointer';
export { FieldErrorMapper, type FieldAliases } from './FieldErrorMapper';
export { CsrfTokenStore } from './CsrfTokenStore';
export { EtagStore, type Versioned } from './EtagStore';
export { CursorIterator, PageLimit, PageParam, type CursorPage, type CursorIteratorOptions } from './CursorIterator';
export { WsClient, WsMessageHandler, WsState, WsUrl, ReconnectPolicy, type WsMessage, type SocketLike, type SocketFactory } from './WsClient';
export { HttpContext, HttpRuntime, ApiRoot, type HttpContextOptions } from './HttpContext';
export { HttpServiceModule, ServiceKeys, type ServiceKey } from './HttpServiceModule';
export { HttpServicesFactory } from './HttpServicesFactory';
export {
  SessionService, AuthMode, AuthEndpoint, AuthProblemCode, ReturnToParam,
  type AuthConfig, type SessionUser, type TeamRole, type PasswordCredentials, type PasswordChangeBody,
} from './session/SessionService';
export { HttpSessionService } from './session/HttpSessionService';
export { HttpMembersService, HttpMembersModule } from './session/HttpMembersModule';
export { CurrentUserMapper } from './session/CurrentUserMapper';
