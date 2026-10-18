import { SegmentedControl, SegmentSize } from '../../../../components/ui';
import { BranchSet, FilterMode } from '../../../../domain';
import { BuilderCommands } from '../../model/BuilderCommands';
import { FilterModeRules } from '../../model/FilterModeRules';
import { FilterWhy } from '../../model/FilterWhy';
import { StepHeading } from '../StepHeading';
import type { StepEditorProps } from '../StepEditorProps';
import { CodeFilterEditor } from './CodeFilterEditor';
import { ExpressionEditor } from './ExpressionEditor';
import { FilterRowList } from './FilterRowList';
import { MatchCounter } from './MatchCounter';
import { WhyBanner } from './WhyBanner';
import styles from './FilterStep.module.css';

/** Filter step (spec §7.2): nested visual conditions, a JS expression or a JS function (Code), why-filtered and the match counter. */
export function FilterStep({ ctx, commands }: StepEditorProps) {
  const { draft, editor, preview, whyIndex, live } = ctx;
  const filter = draft.filter;
  const changeMode = (mode: FilterMode) => {
    if (mode === filter.mode) return;
    const patch = FilterModeRules.switchTo(filter, mode);
    if (patch) editor.updateFilter(() => patch, `Filter switched to ${mode}`);
    else commands.notify(BuilderCommands.modeMessage(mode));
  };
  const why = whyIndex === null ? null : preview.filterResults[whyIndex];
  const results = FilterWhy.results(filter, live.state.records, live.state.result, whyIndex, BranchSet.at(draft));
  const expressionError = preview.filterResults.find((r) => r.error)?.error ?? '';
  return (
    <div className={styles.step}>
      <StepHeading
        title="Filter"
        subtitle="Events must match these conditions to continue."
        actions={<SegmentedControl options={FilterModeRules.options} value={filter.mode} onChange={changeMode} size={SegmentSize.Sm} ariaLabel="Filter mode" />}
      />
      {why && whyIndex !== null && <WhyBanner index={whyIndex} passed={why.pass} onClear={() => ctx.setWhyIndex(null)} />}
      {filter.mode === FilterMode.Visual && <FilterRowList filter={filter} catalog={ctx.catalog} editor={editor} results={results} />}
      {filter.mode === FilterMode.Expression && (
        <ExpressionEditor
          value={filter.expression}
          error={expressionError}
          onChange={(expression) => editor.updateFilter(() => ({ expression }), 'Filter expression edited')}
        />
      )}
      {filter.mode === FilterMode.Code && (
        <CodeFilterEditor
          value={filter.expression}
          error={expressionError}
          onChange={(expression) => editor.updateFilter(() => ({ expression }), 'Filter function edited')}
        />
      )}
      <MatchCounter pass={preview.passIndexes.length} total={preview.total} pending={!preview.ready && preview.total > 0} />
    </div>
  );
}
