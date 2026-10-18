import type { OpName } from '../../api/generated';
import { FilterOp } from '../filter';

/** How a visual operator's typed value is stored in the route document. */
export const OperandShape = { Text: 'text', Number: 'number', List: 'list', None: 'none' } as const;
export type OperandShape = (typeof OperandShape)[keyof typeof OperandShape];

type OperatorEntry = { op: OpName; shape: OperandShape };

/** Maps the builder's visual operators to engine operators and back (by operand type). */
export class OperatorCatalog {
  static readonly listSeparator = ',';

  private static readonly toEngine: Record<FilterOp, OperatorEntry> = {
    [FilterOp.Equals]: { op: 'eq', shape: OperandShape.Text },
    [FilterOp.NotEquals]: { op: 'ne', shape: OperandShape.Text },
    [FilterOp.Contains]: { op: 'contains', shape: OperandShape.Text },
    [FilterOp.StartsWith]: { op: 'starts_with', shape: OperandShape.Text },
    [FilterOp.InList]: { op: 'in', shape: OperandShape.List },
    [FilterOp.Exists]: { op: 'exists', shape: OperandShape.None },
    [FilterOp.NumEquals]: { op: 'eq', shape: OperandShape.Number },
    [FilterOp.NumNotEquals]: { op: 'ne', shape: OperandShape.Number },
    [FilterOp.GreaterThan]: { op: 'gt', shape: OperandShape.Number },
    [FilterOp.LessThan]: { op: 'lt', shape: OperandShape.Number },
  };

  static engine(op: FilterOp): OperatorEntry {
    return OperatorCatalog.toEngine[op];
  }

  /** The visual operator for an engine operator and operand, or null when the builder cannot show it. */
  static visual(op: OpName, operand: unknown): FilterOp | null {
    const numeric = typeof operand === 'number';
    const shape = numeric ? OperandShape.Number : OperatorCatalog.shapeOf(op);
    const match = (Object.entries(OperatorCatalog.toEngine) as [FilterOp, OperatorEntry][]).find(
      ([, entry]) => entry.op === op && entry.shape === shape,
    );
    return match ? match[0] : null;
  }

  private static shapeOf(op: OpName): OperandShape {
    if (op === 'in') return OperandShape.List;
    if (op === 'exists') return OperandShape.None;
    return OperandShape.Text;
  }
}
