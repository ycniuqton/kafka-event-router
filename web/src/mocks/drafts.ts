import { FilterOp, MaskMode, StepFactory, TransformStepKind, type DraftPreset, type FilterRow } from '../domain';

const eventTypeIs = (value: string): FilterRow => ({ field: 'value.eventType', op: FilterOp.Equals, value });

/** Filter rows and transform steps the builder opens with for each seeded route (design `configFor`). */
export class DraftPresets {
  static forRoute(routeId: string): DraftPreset {
    const preset = DraftPresets.byRoute[routeId];
    return preset ? preset() : { rows: [eventTypeIs('OrderPaid')], steps: [] };
  }

  private static readonly byRoute: Record<string, () => DraftPreset> = {
    r1: () => ({
      rows: [eventTypeIs('OrderPaid'), { field: 'value.amount', op: FilterOp.GreaterThan, value: '100' }],
      steps: [
        StepFactory.create(TransformStepKind.Rename, { field: 'cust_id', argument: 'customerId' }),
        StepFactory.create(TransformStepKind.Mask, { field: 'email', maskMode: MaskMode.Email }),
        StepFactory.create(TransformStepKind.Remove, { field: 'items' }),
        StepFactory.create(TransformStepKind.Set, { field: 'source', argument: 'partner-feed' }),
        StepFactory.create(TransformStepKind.SetKey, { field: 'orderId' }),
      ],
    }),
    r3: () => ({
      rows: [eventTypeIs('OrderPaid')],
      steps: [StepFactory.create(TransformStepKind.Js), StepFactory.create(TransformStepKind.Remove, { field: 'email' })],
    }),
    r2: () => ({ rows: [eventTypeIs('OrderRefunded')], steps: [] }),
    r7: () => ({ rows: [eventTypeIs('OrderShipped')], steps: [StepFactory.create(TransformStepKind.Mask, { field: 'email' })] }),
  };
}
