import { useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { QueryParam } from '../../../app/paths';
import { BuilderStep } from '../../../domain';

/** Parses the `step` query parameter, falling back to the Source step. */
export class BuilderStepParam {
  private static readonly all = Object.values(BuilderStep) as string[];

  static parse(raw: string | null): BuilderStep {
    return raw && BuilderStepParam.all.includes(raw) ? (raw as BuilderStep) : BuilderStep.Source;
  }

  static sample(raw: string | null): number | null {
    return raw === null || raw === '' || Number.isNaN(Number(raw)) ? null : Number(raw);
  }
}

/** Current step, studio sample and attached sample session (`?step=&sample=&session=`), kept in the URL so links land in place. */
export function useBuilderNavigation() {
  const [params, setParams] = useSearchParams();
  const step = BuilderStepParam.parse(params.get(QueryParam.Step));
  const sampleId = BuilderStepParam.sample(params.get(QueryParam.Sample));
  const sessionId = params.get(QueryParam.Session);
  const update = useCallback(
    (key: string, value: string | null) =>
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (value === null) next.delete(key);
          else next.set(key, value);
          return next;
        },
        { replace: true },
      ),
    [setParams],
  );
  const goTo = useCallback((s: BuilderStep) => update(QueryParam.Step, s), [update]);
  const selectSample = useCallback((id: number | null) => update(QueryParam.Sample, id === null ? null : String(id)), [update]);
  return { step, sampleId, sessionId, goTo, selectSample };
}
