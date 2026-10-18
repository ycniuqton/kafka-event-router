import { FilterCombinator, FilterGroups, FilterLiteral, FilterMode, FilterOp, type FilterGroup, type FilterRow } from '../filter';

type Token = { kind: TokenKind; text: string };

/** Token classes of the expression subset the visual builder can show. */
const TokenKind = { Str: 'str', Num: 'num', Ident: 'ident', Op: 'op', End: 'end' } as const;
type TokenKind = (typeof TokenKind)[keyof typeof TokenKind];

type ParsedNode = { row: FilterRow } | { combinator: FilterCombinator; children: ParsedNode[] };

/** Tokenizer of filter expressions: strings, numbers, identifiers and the operators the builder emits. */
class ExpressionLexer {
  private static readonly pattern = /\s*(?:("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')|(-?\d+(?:\.\d+)?)|([A-Za-z_$][\w$]*)|(===|!==|==|!=|&&|\|\||>=|<=|[><()[\],.]))/y;

  static tokens(source: string): Token[] | null {
    const out: Token[] = [];
    const re = new RegExp(ExpressionLexer.pattern.source, 'y');
    while (re.lastIndex < source.trimEnd().length) {
      const m = re.exec(source);
      if (!m) return null;
      out.push(ExpressionLexer.token(m));
    }
    out.push({ kind: TokenKind.End, text: '' });
    return out;
  }

  private static token(m: RegExpExecArray): Token {
    if (m[1] !== undefined) return { kind: TokenKind.Str, text: m[1] };
    if (m[2] !== undefined) return { kind: TokenKind.Num, text: m[2] };
    if (m[3] !== undefined) return { kind: TokenKind.Ident, text: m[3] };
    return { kind: TokenKind.Op, text: m[4] };
  }
}

/** Comparison operators by literal kind; `undefined` comparisons become Exists. */
class ComparisonTable {
  static readonly equal = new Set(['===', '==']);
  static readonly notEqual = new Set(['!==', '!=']);
  static readonly undefinedWord = 'undefined';
  static readonly eventRoot = 'event';
  static readonly includes = 'includes';
  static readonly startsWith = 'startsWith';
  static readonly literals: ReadonlySet<string> = new Set(Object.values(FilterLiteral));

  static op(operator: string, literal: Token): { op: FilterOp; literal?: boolean } | null {
    const numeric = literal.kind === TokenKind.Num;
    const isLiteral = literal.kind === TokenKind.Ident && ComparisonTable.literals.has(literal.text);
    if (literal.kind === TokenKind.Ident && literal.text === ComparisonTable.undefinedWord) {
      return ComparisonTable.notEqual.has(operator) ? { op: FilterOp.Exists } : null;
    }
    if (literal.kind === TokenKind.Ident && !isLiteral) return null;
    if (ComparisonTable.equal.has(operator)) return { op: numeric ? FilterOp.NumEquals : FilterOp.Equals, literal: isLiteral || undefined };
    if (ComparisonTable.notEqual.has(operator)) return { op: numeric ? FilterOp.NumNotEquals : FilterOp.NotEquals, literal: isLiteral || undefined };
    if (!numeric) return null;
    if (operator === '>') return { op: FilterOp.GreaterThan };
    return operator === '<' ? { op: FilterOp.LessThan } : null;
  }

  static text(token: Token): string {
    if (token.kind !== TokenKind.Str) return token.text;
    const body = token.text.slice(1, -1);
    return token.text.startsWith('"') ? (JSON.parse(token.text) as string) : body.replace(/\\(.)/g, '$1');
  }
}

/** Recursive-descent parser: `||` of `&&` of comparisons or parenthesised groups. */
class ExpressionGrammar {
  private readonly tokens: Token[];
  private at = 0;

  constructor(tokens: Token[]) {
    this.tokens = tokens;
  }

  parse(): ParsedNode | null {
    const node = this.binary('||', FilterCombinator.Or);
    return node && this.peek().kind === TokenKind.End ? node : null;
  }

  private binary(operator: string, combinator: FilterCombinator): ParsedNode | null {
    const next = () => (operator === '||' ? this.binary('&&', FilterCombinator.And) : this.unary());
    const children: ParsedNode[] = [];
    do {
      const child = next();
      if (!child) return null;
      children.push(child);
    } while (this.accept(operator));
    return children.length === 1 ? children[0] : { combinator, children };
  }

  private unary(): ParsedNode | null {
    if (!this.accept('(')) return this.comparison();
    const inner = this.binary('||', FilterCombinator.Or);
    return inner && this.accept(')') ? inner : null;
  }

  private comparison(): ParsedNode | null {
    if (this.accept('[')) return this.inList();
    const path = this.path();
    if (!path) return null;
    if (path.method) return this.methodCall(path.field, path.method);
    const operator = this.next();
    const literal = this.next();
    const op = operator.kind === TokenKind.Op ? ComparisonTable.op(operator.text, literal) : null;
    if (!op || !path.field) return null;
    const value = op.op === FilterOp.Exists ? '' : ComparisonTable.text(literal);
    return { row: op.literal ? { field: path.field, op: op.op, value, literal: true } : { field: path.field, op: op.op, value } };
  }

  private methodCall(field: string, method: string): ParsedNode | null {
    const op = method === ComparisonTable.includes ? FilterOp.Contains : method === ComparisonTable.startsWith ? FilterOp.StartsWith : null;
    const arg = this.next();
    if (!op || arg.kind !== TokenKind.Str || !this.accept(')')) return null;
    return { row: { field, op, value: ComparisonTable.text(arg) } };
  }

  private inList(): ParsedNode | null {
    const values: string[] = [];
    do {
      const item = this.next();
      if (item.kind !== TokenKind.Str) return null;
      values.push(ComparisonTable.text(item));
    } while (this.accept(','));
    if (!this.accept(']') || !this.accept('.') || this.next().text !== ComparisonTable.includes || !this.accept('(')) return null;
    const path = this.path();
    if (!path?.field || path.method || !this.accept(')')) return null;
    return { row: { field: path.field, op: FilterOp.InList, value: values.join(', ') } };
  }

  /** `event.a.b` → field `a.b`; a trailing `.includes(` / `.startsWith(` is returned as the method. */
  private path(): { field: string; method?: string } | null {
    if (this.next().text !== ComparisonTable.eventRoot) return null;
    const parts: string[] = [];
    while (this.accept('.')) {
      const ident = this.next();
      if (ident.kind !== TokenKind.Ident) return null;
      parts.push(ident.text);
    }
    if (this.accept('(')) return { field: parts.slice(0, -1).join('.'), method: parts[parts.length - 1] };
    return { field: parts.join('.') };
  }

  private peek(): Token {
    return this.tokens[this.at];
  }

  private next(): Token {
    const token = this.tokens[this.at];
    if (token.kind !== TokenKind.End) this.at += 1;
    return token;
  }

  private accept(text: string): boolean {
    if (this.peek().kind !== TokenKind.Op || this.peek().text !== text) return false;
    this.at += 1;
    return true;
  }
}

/**
 * Expression → visual rows (FR-15): succeeds only for the subset the builder emits (comparisons,
 * includes/startsWith, list membership, `&&`/`||` with parentheses up to the nesting limit); otherwise null.
 */
export class ExpressionParser {
  static readonly matchAll = 'true';

  static toGroup(expression: string): FilterGroup | null {
    const source = expression.trim();
    const empty: FilterGroup = { combinator: FilterCombinator.And, rows: [], mode: FilterMode.Visual, expression: '', groups: [] };
    if (!source || source === ExpressionParser.matchAll) return empty;
    const tokens = ExpressionLexer.tokens(source);
    const tree = tokens ? new ExpressionGrammar(tokens).parse() : null;
    if (!tree) return null;
    return ExpressionParser.group('row' in tree ? { combinator: FilterCombinator.And, children: [tree] } : tree, 1);
  }

  private static group(node: { combinator: FilterCombinator; children: ParsedNode[] }, depth: number): FilterGroup | null {
    if (depth > FilterGroups.maxDepth) return null;
    const group: FilterGroup = { combinator: node.combinator, rows: [], mode: FilterMode.Visual, expression: '', groups: [] };
    for (const child of ExpressionParser.flatten(node)) {
      const nested = 'row' in child ? null : ExpressionParser.group(child, depth + 1);
      if ('row' in child) group.rows.push(child.row);
      else if (nested) group.groups?.push(nested);
      else return null;
    }
    return group;
  }

  /** Merges children with the same combinator into their parent: `(a && b) && c` → `a && b && c`. */
  private static flatten(node: { combinator: FilterCombinator; children: ParsedNode[] }): ParsedNode[] {
    return node.children.flatMap((c) => ('combinator' in c && c.combinator === node.combinator ? ExpressionParser.flatten(c) : [c]));
  }
}
