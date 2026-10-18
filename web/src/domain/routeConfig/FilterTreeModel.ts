import type { FilterNode } from '../../api/generated';
import { FilterCombinator, FilterGroups, FilterMode, type FilterGroup, type FilterRow } from '../filter';
import { FilterCode } from '../pipeline/FilterCode';
import { OperandShape, OperatorCatalog } from './OperatorCatalog';

/** Result of reading an AST into the builder: `exact` is false when it fell back to expression mode. */
export type FilterReadResult = { group: FilterGroup; exact: boolean };

/** Where each visual condition lives in the AST: JSON Pointers of top-level rows and of nested group rows. */
export type FilterPointers = { rows: string[]; groups: FilterPointers[]; group: string };

type GroupNodeType = 'and' | 'or';

/**
 * Converts between the builder's (nested) filter group and the engine filter AST (`and|or|not|cond|expr`).
 * A group with a single part collapses to that part, so pointers follow the same rule.
 */
export class FilterTreeModel {
  static readonly unsupportedNotice = '/* This filter uses conditions the visual builder cannot show. */';
  static readonly branchFilter = '/branches/0/filter';
  static readonly childrenToken = 'children';

  private static readonly combinatorNode: Record<FilterCombinator, GroupNodeType> = {
    [FilterCombinator.And]: 'and',
    [FilterCombinator.Or]: 'or',
  };

  /** Builder group → AST; an empty visual group means "keep every event" (no filter). */
  static toNode(group: FilterGroup): FilterNode | undefined {
    if (group.mode === FilterMode.Expression) {
      const code = group.expression.trim();
      return code ? { type: 'expr', code } : undefined;
    }
    if (group.mode === FilterMode.Code) {
      const fn = group.expression.trim();
      return fn ? { type: 'expr', code: FilterCode.wrap(fn) } : undefined;
    }
    return FilterTreeModel.groupNode(group);
  }

  static cond(row: FilterRow): FilterNode {
    const entry = OperatorCatalog.engine(row.op);
    const node: FilterNode = { type: 'cond', field: row.field, op: entry.op };
    if (entry.shape === OperandShape.Number) node.value = Number(row.value);
    if (entry.shape === OperandShape.Text) node.value = FilterGroups.literalValue(row);
    if (entry.shape === OperandShape.List) node.values = row.value.split(OperatorCatalog.listSeparator).map((v) => v.trim());
    return node;
  }

  /** AST → builder group: nested and/or of supported conditions stays visual, `expr` and the rest become expressions. */
  static fromNode(node: FilterNode | undefined): FilterReadResult {
    const empty: FilterGroup = { combinator: FilterCombinator.And, rows: [], mode: FilterMode.Visual, expression: '', groups: [] };
    if (!node) return { group: empty, exact: true };
    if (node.type === 'expr') return { group: FilterTreeModel.exprGroup(empty, node.code ?? ''), exact: true };
    const group = FilterTreeModel.readGroup(node, 1);
    if (group) return { group: { ...group, mode: FilterMode.Visual, expression: '' }, exact: true };
    return { group: { ...empty, mode: FilterMode.Expression, expression: FilterTreeModel.unsupportedNotice }, exact: false };
  }

  /** An `expr` node: Code mode when it is a wrapped function, else Expression mode. */
  private static exprGroup(empty: FilterGroup, code: string): FilterGroup {
    const fn = FilterCode.unwrap(code);
    return fn === null ? { ...empty, mode: FilterMode.Expression, expression: code } : { ...empty, mode: FilterMode.Code, expression: fn };
  }

  /** JSON Pointers of every visual condition (same collapse rule as `toNode`), for why-filtered traces. */
  static pointers(group: FilterGroup, at: string = FilterTreeModel.branchFilter): FilterPointers {
    const nested = FilterGroups.children(group).filter((g) => !FilterGroups.isEmpty(g));
    const parts = group.rows.length + nested.length;
    const child = (i: number) => (parts === 1 ? at : `${at}/${FilterTreeModel.childrenToken}/${i}`);
    const groupPointers = nested.map((g, i) => FilterTreeModel.pointers(g, child(group.rows.length + i)));
    const byGroup = new Map(nested.map((g, i) => [g, groupPointers[i]]));
    return {
      group: at,
      rows: group.rows.map((_, i) => child(i)),
      groups: FilterGroups.children(group).map((g) => byGroup.get(g) ?? { group: at, rows: [], groups: [] }),
    };
  }

  private static groupNode(group: FilterGroup): FilterNode | undefined {
    const nested = FilterGroups.children(group)
      .map((g) => FilterTreeModel.groupNode(g))
      .filter((n): n is FilterNode => n !== undefined);
    const children = [...group.rows.map((row) => FilterTreeModel.cond(row)), ...nested];
    if (!children.length) return undefined;
    return children.length === 1 ? children[0] : { type: FilterTreeModel.combinatorNode[group.combinator], children };
  }

  private static readGroup(node: FilterNode, depth: number): FilterGroup | null {
    const isGroup = node.type === 'and' || node.type === 'or';
    const combinator = node.type === 'or' ? FilterCombinator.Or : FilterCombinator.And;
    const group: FilterGroup = { combinator, rows: [], mode: FilterMode.Visual, expression: '', groups: [] };
    for (const child of isGroup ? (node.children ?? []) : [node]) {
      if (!FilterTreeModel.addChild(group, child, depth)) return null;
    }
    return group;
  }

  private static addChild(group: FilterGroup, child: FilterNode, depth: number): boolean {
    if (child.type === 'and' || child.type === 'or') {
      const nested = depth < FilterGroups.maxDepth ? FilterTreeModel.readGroup(child, depth + 1) : null;
      if (nested) group.groups?.push(nested);
      return nested !== null;
    }
    const row = FilterTreeModel.row(child);
    if (row) group.rows.push(row);
    return row !== null;
  }

  private static row(node: FilterNode): FilterRow | null {
    if (node.type !== 'cond' || !node.field || !node.op) return null;
    const operand = node.op === 'in' ? node.values : node.value;
    const op = OperatorCatalog.visual(node.op, operand);
    if (!op) return null;
    const literal = typeof operand === 'boolean' || operand === null;
    const value = Array.isArray(operand) ? operand.join(`${OperatorCatalog.listSeparator} `) : operand === undefined ? '' : String(operand);
    return literal ? { field: node.field, op, value, literal } : { field: node.field, op, value };
  }
}
