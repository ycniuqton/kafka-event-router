import type { NavigateFunction } from 'react-router-dom';
import { AppPaths, QueryParam } from '../../../app/paths';
import { BuilderStep, FilterMode, RouteDetailTab, type RouteDraft } from '../../../domain';
import { actions } from '../../../store';
import type { DraftEditor } from './DraftSession';
import { BuilderStepParam } from './useBuilderNavigation';
import type { FilterFieldCatalog } from './FilterFieldCatalog';
import { TestRunReport } from '../../../components/sample-panel';
import type { BuilderPreview } from './useBuilderPreview';

/** Builder actions that leave the draft: save, test run, deploy, navigation and quick filters. */
export class BuilderCommands {
  static readonly saveFailedPrefix = 'Draft not saved: ';
  static readonly noSamplesMessage = 'Load samples first to run a test.';
  static readonly visualOnlyMessage = "This expression can't be converted back to visual rows";
  static readonly expressionOnlyMessage = "This function can't be converted to an expression: keep it to a single return";

  private readonly navigate: NavigateFunction;
  private readonly editor: DraftEditor;

  constructor(navigate: NavigateFunction, editor: DraftEditor) {
    this.navigate = navigate;
    this.editor = editor;
  }

  /** Saves the draft; the first save of a new route moves the URL to its edit page (same step). */
  async save(draft: RouteDraft): Promise<void> {
    const saved = await actions.routes.saveDraft(draft).catch((error: Error) => {
      actions.toast.show(`${BuilderCommands.saveFailedPrefix}${error.message}`);
      return null;
    });
    if (!saved) return;
    this.editor.markSaved(saved);
    const step = BuilderStepParam.parse(new URLSearchParams(window.location.search).get(QueryParam.Step));
    if (!draft.routeId && saved.routeId) this.navigate(AppPaths.routeEdit(saved.routeId, step), { replace: true });
  }

  /** Test button / Ctrl+Enter: re-runs the draft now, then the full route with output validation. */
  async test(live: BuilderPreview): Promise<void> {
    if (!live.state.session) return actions.toast.show(BuilderCommands.noSamplesMessage);
    await live.actions.runNow(live.config);
    const result = await live.actions.test(live.config);
    if (result) actions.toast.show(TestRunReport.toast(result));
  }

  /** Deploys and lands on the overview (History with the pending request when approval is needed); `overwrite` first takes the newest ETag (after a 412). */
  async deploy(draft: RouteDraft, note: string, overwrite = false): Promise<void> {
    if (overwrite && draft.routeId) await actions.routes.refresh(draft.routeId);
    const { route, pending } = await actions.routes.deploy(draft, note);
    this.navigate(AppPaths.routeDetail(route.id, pending ? RouteDetailTab.History : RouteDetailTab.Overview));
  }

  /** Discards local edits and re-opens the stored draft (after a concurrent save). */
  reload(): void {
    window.location.reload();
  }

  back(draft: RouteDraft): void {
    this.navigate(draft.routeId ? AppPaths.routeDetail(draft.routeId) : AppPaths.routes());
  }

  /** Toast text when switching the filter to mode is not possible. */
  static modeMessage(mode: FilterMode): string {
    return mode === FilterMode.Expression ? BuilderCommands.expressionOnlyMessage : BuilderCommands.visualOnlyMessage;
  }

  notify(message: string): void {
    actions.toast.show(message);
  }

  /** "Keep only events where field = value": adds a visual row and opens the Filter step. */
  keepOnly(catalog: FilterFieldCatalog, field: string, value: unknown, goTo: (step: BuilderStep) => void): void {
    const row = catalog.rowFromValue(field, value);
    this.editor.updateFilter((f) => ({ mode: FilterMode.Visual, rows: [...f.rows, row] }), `Added filter ${field}`);
    goTo(BuilderStep.Filter);
    actions.toast.show(`Added: ${field} = ${String(value)}`);
  }
}
