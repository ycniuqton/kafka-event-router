import { useEffect, useMemo, useRef, useState } from 'react';
import { services } from '../../../../api';
import type { TransformStep } from '../../../../domain';
import type { StudioRunResult } from '../preview/StudioRunResult';
import { TestCaseController, type TestCaseState } from './TestCaseController';

/** Test cases of the route (null route: none); re-run after every successful preview of the steps. */
export function useTestCases(routeId: string | null, steps: TransformStep[], run: StudioRunResult | null): { state: TestCaseState; controller: TestCaseController | null } {
  const [state, setState] = useState<TestCaseState>(TestCaseController.initial);
  const controller = useMemo(() => (routeId ? new TestCaseController(services.testCases, routeId, setState) : null), [routeId]);
  useEffect(() => {
    if (!controller) return;
    controller.activate();
    void controller.load();
    return () => controller.dispose();
  }, [controller]);
  const latest = useRef(steps);
  useEffect(() => {
    latest.current = steps;
  }, [steps]);
  useEffect(() => {
    if (controller && run) void controller.rerun(latest.current);
  }, [controller, run]);
  return { state, controller };
}
