import type { Services } from '../api';
import { BulkAction, RouteStatus, type BulkItemResult, type Route, type RouteDraft, type RouteSettings, type RouteSettingsPatch, type RouteVersion } from '../domain';
import { DeploymentStatus, type DocumentFormat, type ImportOptions, type ImportReport, type RouteExport } from '../domain';
import type { AppStore } from './AppStore';
import type { ToastController } from './ToastController';

/** Route lifecycle actions: pause/resume, duplicate, archive, drafts, deploy, history and rollback. */
export class RouteActions {
  private readonly store: AppStore;
  private readonly services: Services;
  private readonly toast: ToastController;

  constructor(store: AppStore, services: Services, toast: ToastController) {
    this.store = store;
    this.services = services;
    this.toast = toast;
  }

  async togglePause(id: string): Promise<void> {
    const route = this.find(id);
    if (!route) return;
    const resuming = route.status === RouteStatus.Paused;
    const updated = resuming ? await this.services.routes.resume(id) : await this.services.routes.pause(id);
    this.replace(updated);
    this.toast.show(resuming ? `Resumed “${route.name}”` : `Paused “${route.name}”`);
  }

  async bulkPause(ids: string[]): Promise<void> {
    const running = ids.filter((id) => this.find(id)?.status === RouteStatus.Running);
    const results = await this.services.routes.bulk(BulkAction.Pause, running);
    this.applyStatus(results, RouteStatus.Paused);
    this.toast.show(BulkOutcome.message(`Paused ${this.okCount(results)} routes`, results));
  }

  async bulkResume(ids: string[]): Promise<void> {
    const paused = ids.filter((id) => this.find(id)?.status === RouteStatus.Paused);
    const results = await this.services.routes.bulk(BulkAction.Resume, paused);
    this.applyStatus(results, RouteStatus.Running);
    this.toast.show(BulkOutcome.message(`Resumed ${this.okCount(results)} routes`, results));
  }

  async bulkTag(ids: string[], tag: string): Promise<void> {
    const results = await this.services.routes.bulk(BulkAction.AddTag, ids, tag);
    this.toast.show(BulkOutcome.message(`Tagged ${this.okCount(results)} routes “${tag}”`, results));
  }

  private applyStatus(results: BulkItemResult[], status: RouteStatus): void {
    const ok = new Set(results.filter((r) => r.ok).map((r) => r.id));
    this.store.setState((s) => ({ routes: s.routes.map((r) => (ok.has(r.id) ? { ...r, status } : r)) }));
  }

  private okCount(results: BulkItemResult[]): number {
    return results.filter((r) => r.ok).length;
  }

  async duplicate(id: string): Promise<Route> {
    const copy = await this.services.routes.duplicate(id);
    this.store.setState((s) => ({ routes: [copy, ...s.routes] }));
    this.toast.show('Duplicated as draft');
    return copy;
  }

  async archive(id: string): Promise<void> {
    const route = this.find(id);
    await this.services.routes.archive(id);
    this.store.setState((s) => ({ routes: s.routes.filter((r) => r.id !== id) }));
    this.toast.show(`Archived “${route?.name ?? id}”`);
  }

  async saveDraft(draft: RouteDraft): Promise<RouteDraft> {
    const saved = await this.services.routes.saveDraft(draft);
    this.toast.show('Draft saved');
    return saved;
  }

  /** Deploys a draft; updates routes, activity and history; returns the route and whether approval is pending. */
  async deploy(draft: RouteDraft, note: string): Promise<{ route: Route; pending: boolean }> {
    const { route, version, activity, deployment } = await this.services.routes.deploy(draft, note);
    this.store.setState((s) => ({
      routes: s.routes.some((r) => r.id === route.id) ? s.routes.map((r) => (r.id === route.id ? route : r)) : [route, ...s.routes],
      activity: [activity, ...s.activity],
      versions: { ...s.versions, [route.id]: [version, ...(s.versions[route.id] ?? [])] },
    }));
    this.toast.show(DeployOutcome.message(version.version, deployment?.status));
    return { route, pending: DeployOutcome.isPending(deployment?.status) };
  }

  async loadVersions(routeId: string): Promise<RouteVersion[]> {
    const versions = await this.services.routes.listVersions(routeId);
    this.store.setState((s) => ({ versions: { ...s.versions, [routeId]: versions } }));
    return versions;
  }

  /** Redeploys an old version as a new one, then refreshes the route and its history from the server. */
  async rollback(routeId: string, version: number): Promise<RouteVersion> {
    const created = await this.services.routes.rollback(routeId, version);
    this.store.setState((s) => ({ versions: { ...s.versions, [routeId]: [created, ...(s.versions[routeId] ?? [])] } }));
    this.toast.show(`Redeployed v${version} as v${created.version}`);
    await Promise.all([this.loadVersions(routeId), this.refresh(routeId)]);
    return created;
  }

  /** Re-reads one route (status, version, names) into the store; drops it when it no longer exists. */
  async refresh(routeId: string): Promise<Route | undefined> {
    const route = await this.services.routes.get(routeId);
    if (route) this.upsert(route);
    else this.store.setState((s) => ({ routes: s.routes.filter((r) => r.id !== routeId) }));
    return route;
  }

  /** Saves settings (name, description, tags, replicas, error policy) and updates the stored route. */
  async saveSettings(routeId: string, settings: RouteSettingsPatch): Promise<RouteSettings> {
    const saved = await this.services.routes.updateSettings(routeId, settings);
    await this.refresh(routeId);
    this.toast.show('Settings saved');
    return saved;
  }

  /** Brings an archived route back (paused) and adds it to the live list. */
  async unarchive(id: string): Promise<Route> {
    const route = await this.services.routes.unarchive(id);
    this.upsert(route);
    this.toast.show(`Unarchived “${route.name}” · paused`);
    return route;
  }

  /** Builds the export file of the routes; saving it is up to the caller (browser download). */
  async exportRoutes(ids: string[], format: DocumentFormat): Promise<RouteExport> {
    const file = await this.services.routes.exportRoutes(ids, format);
    this.toast.show(`Exported ${ids.length} route${ids.length === 1 ? '' : 's'} as ${format.toUpperCase()}`);
    return file;
  }

  /** Imports documents as drafts; a real run reloads the routes list. */
  async importRoutes(content: string, options: ImportOptions): Promise<ImportReport> {
    const report = await this.services.routes.importRoutes(content, options);
    if (!report.dryRun) {
      const routes = await this.services.routes.list();
      this.store.setState(() => ({ routes }));
    }
    return report;
  }

  private find(id: string): Route | undefined {
    return this.store.getState().routes.find((r) => r.id === id);
  }

  private upsert(route: Route): void {
    this.store.setState((s) => ({ routes: s.routes.some((r) => r.id === route.id) ? s.routes.map((r) => (r.id === route.id ? route : r)) : [route, ...s.routes] }));
  }

  private replace(route: Route): void {
    this.store.setState((s) => ({ routes: s.routes.map((r) => (r.id === route.id ? route : r)) }));
  }
}

/** Toast of a deploy: live now, or waiting for a second person (approval requested). */
export class DeployOutcome {
  static message(versionNo: number, status: DeploymentStatus | undefined): string {
    if (status === DeploymentStatus.PendingApproval) return `Approval requested for v${versionNo}`;
    return `Deployed v${versionNo} · running`;
  }

  static isPending(status: DeploymentStatus | undefined): boolean {
    return status === DeploymentStatus.PendingApproval;
  }
}

/** Summarises per-route bulk results for the toast: successes plus the first failure, if any. */
export class BulkOutcome {
  static message(success: string, results: BulkItemResult[]): string {
    const failed = results.filter((r) => !r.ok);
    if (!failed.length) return success;
    return `${success} · ${failed.length} failed: ${failed[0].message ?? failed[0].id}`;
  }
}
