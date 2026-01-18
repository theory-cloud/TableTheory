export interface AggregateResult {
  min?: unknown;
  max?: unknown;
  count: number;
  sum: number;
  average: number;
}

export interface GroupedResult<T = Record<string, unknown>> {
  key: unknown;
  count: number;
  items: T[];
  aggregates: Record<string, AggregateResult>;
}

type AggregateFunction = 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX';

interface AggregateOp {
  function: AggregateFunction;
  field: string;
  alias: string;
}

interface HavingClause {
  aggregate: string;
  operator: string;
  value: unknown;
}

export class GroupByQuery<T extends Record<string, unknown>> {
  private readonly aggregates: AggregateOp[] = [];
  private readonly havingClauses: HavingClause[] = [];

  constructor(
    private readonly items: () => Promise<T[]>,
    private readonly groupByField: string,
  ) {}

  count(alias: string): this {
    this.aggregates.push({ function: 'COUNT', field: '*', alias });
    return this;
  }

  sum(field: string, alias: string): this {
    this.aggregates.push({ function: 'SUM', field, alias });
    return this;
  }

  avg(field: string, alias: string): this {
    this.aggregates.push({ function: 'AVG', field, alias });
    return this;
  }

  min(field: string, alias: string): this {
    this.aggregates.push({ function: 'MIN', field, alias });
    return this;
  }

  max(field: string, alias: string): this {
    this.aggregates.push({ function: 'MAX', field, alias });
    return this;
  }

  having(aggregate: string, operator: string, value: unknown): this {
    this.havingClauses.push({ aggregate, operator, value });
    return this;
  }

  async execute(): Promise<Array<GroupedResult<T>>> {
    const items = await this.items();
    const groups = new Map<string, GroupedResult<T>>();

    for (const item of items) {
      const key = extractFieldValue(item, this.groupByField);
      if (key === undefined) continue;

      const keyStr = String(key);
      const group = groups.get(keyStr);
      if (group) {
        group.count += 1;
        group.items.push(item);
      } else {
        groups.set(keyStr, {
          key,
          count: 1,
          items: [item],
          aggregates: {},
        });
      }
    }

    for (const group of groups.values()) {
      for (const op of this.aggregates) {
        group.aggregates[op.alias] = calculateAggregate(group.items, op);
      }
    }

    const out: Array<GroupedResult<T>> = [];
    for (const group of groups.values()) {
      if (evaluateHaving(group, this.havingClauses)) out.push(group);
    }
    return out;
  }
}

export function sumField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): number {
  let sum = 0;
  for (const item of items) {
    const value = extractNumericValue(item, field);
    if (value === undefined) continue;
    sum += value;
  }
  return sum;
}

export function averageField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): number {
  if (items.length === 0) return 0;

  let sum = 0;
  let count = 0;
  for (const item of items) {
    const value = extractNumericValue(item, field);
    if (value === undefined) continue;
    sum += value;
    count += 1;
  }
  if (count === 0) return 0;
  return sum / count;
}

export function minField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): unknown {
  return extremeValue(items, field, -1);
}

export function maxField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): unknown {
  return extremeValue(items, field, 1);
}

export function aggregateField<T extends Record<string, unknown>>(
  items: T[],
  field?: string,
): AggregateResult {
  const result: AggregateResult = {
    count: items.length,
    sum: 0,
    average: 0,
  };

  if (!field) return result;

  let sum = 0;
  let numericCount = 0;
  let min: unknown = undefined;
  let max: unknown = undefined;

  for (const item of items) {
    const num = extractNumericValue(item, field);
    if (num !== undefined) {
      sum += num;
      numericCount += 1;
    }

    const value = extractFieldValue(item, field);
    if (value === undefined) continue;

    if (min === undefined) min = value;
    else if (compareValues(value, min) < 0) min = value;

    if (max === undefined) max = value;
    else if (compareValues(value, max) > 0) max = value;
  }

  result.sum = sum;
  if (numericCount > 0) result.average = sum / numericCount;
  if (min !== undefined) result.min = min;
  if (max !== undefined) result.max = max;
  return result;
}

export function countDistinct<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): number {
  const unique = new Set<string>();
  for (const item of items) {
    const value = extractFieldValue(item, field);
    if (value === undefined) continue;
    unique.add(String(value));
  }
  return unique.size;
}

function calculateAggregate<T extends Record<string, unknown>>(
  items: T[],
  op: AggregateOp,
): AggregateResult {
  const result: AggregateResult = {
    count: 0,
    sum: 0,
    average: 0,
  };

  switch (op.function) {
    case 'COUNT':
      result.count = items.length;
      break;
    case 'SUM':
      result.sum = sumField(items, op.field);
      break;
    case 'AVG':
      result.average = averageField(items, op.field);
      break;
    case 'MIN': {
      const v = extremeFieldValue(items, op.field, false);
      if (v !== undefined) result.min = v;
      break;
    }
    case 'MAX': {
      const v = extremeFieldValue(items, op.field, true);
      if (v !== undefined) result.max = v;
      break;
    }
  }

  return result;
}

function extremeValue<T extends Record<string, unknown>>(
  items: T[],
  field: string,
  direction: -1 | 1,
): unknown {
  if (items.length === 0) {
    throw new Error('no items found');
  }

  let extreme: unknown = undefined;
  for (const item of items) {
    const value = extractFieldValue(item, field);
    if (value === undefined) continue;

    if (extreme === undefined) {
      extreme = value;
      continue;
    }

    const cmp = compareValues(value, extreme);
    if ((direction < 0 && cmp < 0) || (direction > 0 && cmp > 0)) {
      extreme = value;
    }
  }

  if (extreme === undefined) {
    throw new Error(`no valid values found for field ${field}`);
  }
  return extreme;
}

function extremeFieldValue<T extends Record<string, unknown>>(
  items: T[],
  field: string,
  pickMax: boolean,
): unknown {
  let selected: unknown = undefined;
  for (const item of items) {
    const value = extractFieldValue(item, field);
    if (value === undefined) continue;

    if (selected === undefined) {
      selected = value;
      continue;
    }

    const cmp = compareValues(value, selected);
    if ((pickMax && cmp > 0) || (!pickMax && cmp < 0)) {
      selected = value;
    }
  }
  return selected;
}

function evaluateHaving<T extends Record<string, unknown>>(
  group: GroupedResult<T>,
  clauses: HavingClause[],
): boolean {
  for (const clause of clauses) {
    const aggValue = aggregateValue(group, clause.aggregate);
    if (aggValue === undefined) return false;

    const compareValue = toFloat(clause.value);
    if (compareValue === undefined) return false;

    if (!compareHaving(aggValue, clause.operator, compareValue)) return false;
  }
  return true;
}

function aggregateValue<T extends Record<string, unknown>>(
  group: GroupedResult<T>,
  aggregate: string,
): number | undefined {
  if (aggregate === 'COUNT(*)') return group.count;

  const result = group.aggregates[aggregate];
  if (!result) return undefined;

  const value = aggregateResultValue(result);
  if (value === undefined) return undefined;

  return value;
}

function aggregateResultValue(result: AggregateResult): number | undefined {
  if (result.min !== undefined) return toFloat(result.min);
  if (result.max !== undefined) return toFloat(result.max);
  if (result.count !== 0) return result.count;
  if (result.sum !== 0) return result.sum;
  if (result.average !== 0) return result.average;
  return 0;
}

function compareHaving(
  aggValue: number,
  operator: string,
  compareValue: number,
): boolean {
  switch (operator) {
    case '=':
      return aggValue === compareValue;
    case '>':
      return aggValue > compareValue;
    case '>=':
      return aggValue >= compareValue;
    case '<':
      return aggValue < compareValue;
    case '<=':
      return aggValue <= compareValue;
    case '!=':
      return aggValue !== compareValue;
    default:
      return true;
  }
}

function extractNumericValue<T extends Record<string, unknown>>(
  item: T,
  field: string,
): number | undefined {
  const value = item[field];
  return toFloat(value);
}

function extractFieldValue<T extends Record<string, unknown>>(
  item: T,
  field: string,
): unknown {
  const value = item[field];
  if (isZeroValue(value)) return undefined;
  return value;
}

function compareValues(a: unknown, b: unknown): number {
  const aFloat = toFloat(a);
  const bFloat = toFloat(b);
  if (aFloat !== undefined && bFloat !== undefined) {
    if (aFloat < bFloat) return -1;
    if (aFloat > bFloat) return 1;
    return 0;
  }

  if (typeof a === 'string' && typeof b === 'string') {
    if (a < b) return -1;
    if (a > b) return 1;
    return 0;
  }

  const aStr = String(a);
  const bStr = String(b);
  if (aStr < bStr) return -1;
  if (aStr > bStr) return 1;
  return 0;
}

function toFloat(value: unknown): number | undefined {
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) return undefined;
    return value;
  }
  if (typeof value === 'bigint') {
    const converted = Number(value);
    if (!Number.isFinite(converted)) return undefined;
    return converted;
  }
  return undefined;
}

function isZeroValue(value: unknown): boolean {
  if (value === null || value === undefined) return true;

  if (typeof value === 'string') return value.length === 0;
  if (typeof value === 'number') return value === 0;
  if (typeof value === 'bigint') return value === 0n;
  if (typeof value === 'boolean') return value === false;

  if (value instanceof Date) return Number.isNaN(value.getTime());
  if (value instanceof Uint8Array) return value.length === 0;

  if (Array.isArray(value)) return value.length === 0;
  if (value instanceof Map) return value.size === 0;
  if (value instanceof Set) return value.size === 0;

  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) return true;
    return entries.every(([, v]) => isZeroValue(v));
  }

  return false;
}
