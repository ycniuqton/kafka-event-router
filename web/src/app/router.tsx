import type { ComponentType } from 'react';
import { createBrowserRouter, type RouteObject } from 'react-router-dom';
import { AppShell } from './AppShell';
import { AuthPaths, SessionGate } from './auth';
import { NavKey, type RouteHandle } from './navigation';
import { RoutePattern } from './paths';

type PageModule = { default: ComponentType };

/** Lazy-loads a page module whose default export is the page component. */
class LazyPage {
  static from(load: () => Promise<PageModule>): RouteObject['lazy'] {
    return async () => ({ Component: (await load()).default });
  }

  static route(path: string, load: () => Promise<PageModule>, handle: RouteHandle): RouteObject {
    return { path, handle, lazy: LazyPage.from(load) };
  }
}

/** All screens. The builder (`routes/new`, `routes/:id/edit`) is fullscreen: no left nav. */
export const routes: RouteObject[] = [
  { path: AuthPaths.login, lazy: LazyPage.from(() => import('./auth/LoginPage')) },
  {
    element: (
      <SessionGate>
        <AppShell />
      </SessionGate>
    ),
    children: [
      { index: true, handle: { nav: NavKey.Dashboard }, lazy: LazyPage.from(() => import('../pages/dashboard/DashboardPage')) },
      LazyPage.route(RoutePattern.Routes, () => import('../pages/routes/RoutesPage'), { nav: NavKey.Routes }),
      LazyPage.route(RoutePattern.RouteNew, () => import('../pages/builder/BuilderPage'), { nav: NavKey.Routes, fullscreen: true }),
      LazyPage.route(RoutePattern.RouteDetail, () => import('../pages/route-detail/RouteDetailPage'), { nav: NavKey.Routes }),
      LazyPage.route(RoutePattern.RouteEdit, () => import('../pages/builder/BuilderPage'), { nav: NavKey.Routes, fullscreen: true }),
      LazyPage.route(RoutePattern.Connections, () => import('../pages/connections/ConnectionsPage'), { nav: NavKey.Connections }),
      LazyPage.route(RoutePattern.Library, () => import('../pages/library/LibraryPage'), { nav: NavKey.Library }),
      LazyPage.route(RoutePattern.DeadLetters, () => import('../pages/dead-letters/DeadLettersPage'), { nav: NavKey.DeadLetters }),
      LazyPage.route(RoutePattern.Audit, () => import('../pages/audit/AuditPage'), { nav: NavKey.Audit }),
      LazyPage.route(RoutePattern.Settings, () => import('../pages/settings/SettingsPage'), { nav: NavKey.Settings }),
      LazyPage.route(RoutePattern.Approval, () => import('../pages/approvals/ApprovalReviewPage'), { nav: NavKey.Routes }),
      LazyPage.route(RoutePattern.NotFound, () => import('../pages/not-found/NotFoundPage'), {}),
    ],
  },
];

/** React Router v7 behaviours opted into early (silences v6 deprecation warnings). */
export const routerFuture = {
  v7_relativeSplatPath: true,
  v7_fetcherPersist: true,
  v7_normalizeFormMethod: true,
  v7_skipActionErrorRevalidation: true,
} as const;

export const router = createBrowserRouter(routes, { future: routerFuture });
