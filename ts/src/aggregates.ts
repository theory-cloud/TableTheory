import { TheorydbError } from './errors.js';

/** Result of an in-memory client-side aggregation over an already materialized item array. */
export interface AggregateResult {
  min?: unknown;
  max?: unknown;
  count: number;
  sum: number;
  average: number;
}

/** Group produced by `GroupByQuery.execute()` after all source items have been materialized in memory. */
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

/**
 * Client-side group-by helper. `execute()` loads the full source item set into memory,
 * then keeps grouped items and aggregates in memory; use only for bounded result sets.
 */
export class GroupByQuery<T extends Record<string, unknown>> {
  private readonly aggregates: AggregateOp[] = [];
  private readonly havingClauses: HavingClause[] = [];

  constructor(
    private readonly items: () => Promise<T[]>,
    private readonly groupByField: string,
  ) {}

  /** Adds a count aggregate computed in memory after the full source set is materialized. */
  count(alias: string): this {
    this.aggregates.push({ function: 'COUNT', field: '*', alias });
    return this;
  }

  /** Adds a sum aggregate computed in memory after the full source set is materialized. */
  sum(field: string, alias: string): this {
    this.aggregates.push({ function: 'SUM', field, alias });
    return this;
  }

  /** Adds an average aggregate computed in memory after the full source set is materialized. */
  avg(field: string, alias: string): this {
    this.aggregates.push({ function: 'AVG', field, alias });
    return this;
  }

  /** Adds a minimum aggregate computed in memory after the full source set is materialized. */
  min(field: string, alias: string): this {
    this.aggregates.push({ function: 'MIN', field, alias });
    return this;
  }

  /** Adds a maximum aggregate computed in memory after the full source set is materialized. */
  max(field: string, alias: string): this {
    this.aggregates.push({ function: 'MAX', field, alias });
    return this;
  }

  /** Adds an in-memory having filter evaluated after groups and aggregates are materialized. */
  having(aggregate: string, operator: string, value: unknown): this {
    this.havingClauses.push({ aggregate, operator, value });
    return this;
  }

  /** Materializes the full source set, groups it in memory, computes aggregates, and returns all groups. */
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

/** Client-side sum over an already materialized item array; use only for bounded result sets. */
export function sumField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): number {
  const accumulator = new ExactAccumulator();
  for (const item of items) {
    accumulator.add(item[field]);
  }
  return requireExactNumber(accumulator.total());
}

/** Client-side average over an already materialized item array; use only for bounded result sets. */
export function averageField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): number {
  const accumulator = new ExactAccumulator();
  for (const item of items) {
    accumulator.add(item[field]);
  }

  const count = accumulator.count();
  if (count === 0) return 0;

  const total = accumulator.total();
  const { num, den } = decimalFraction(total);
  const average = fractionToNumber(num, den * BigInt(count));
  if (average === undefined) {
    throw precisionLossError(canonicalFromDecimal(total) + '/' + count);
  }
  return average;
}

/** Client-side minimum over an already materialized item array; use only for bounded result sets. */
export function minField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): unknown {
  return extremeValue(items, field, -1);
}

/** Client-side maximum over an already materialized item array; use only for bounded result sets. */
export function maxField<T extends Record<string, unknown>>(
  items: T[],
  field: string,
): unknown {
  return extremeValue(items, field, 1);
}

/** Client-side aggregate over an already materialized item array; use only for bounded result sets. */
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

  const accumulator = new ExactAccumulator();
  let min: unknown = undefined;
  let max: unknown = undefined;

  for (const item of items) {
    accumulator.add(item[field]);

    const value = extractFieldValue(item, field);
    if (value === undefined) continue;

    if (min === undefined) min = value;
    else if (compareValues(value, min) < 0) min = value;

    if (max === undefined) max = value;
    else if (compareValues(value, max) > 0) max = value;
  }

  if (accumulator.count() > 0) {
    const total = accumulator.total();
    result.sum = requireExactNumber(total);

    const { num, den } = decimalFraction(total);
    const average = fractionToNumber(num, den * BigInt(accumulator.count()));
    if (average === undefined) {
      throw precisionLossError(
        canonicalFromDecimal(total) + '/' + accumulator.count(),
      );
    }
    result.average = average;
  }

  if (min !== undefined) result.min = min;
  if (max !== undefined) result.max = max;
  return result;
}

/** Client-side distinct count over an already materialized item array; use only for bounded result sets. */
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

function assertWithinNumberRange(value: unknown): void {
  decimalFromValue(value);
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
    assertWithinNumberRange(value);

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
    assertWithinNumberRange(value);

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

    const compareValue = toExactNumber(clause.value);
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
  if (result.min !== undefined) return toExactNumber(result.min);
  if (result.max !== undefined) return toExactNumber(result.max);
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

function extractFieldValue<T extends Record<string, unknown>>(
  item: T,
  field: string,
): unknown {
  const value = item[field];
  if (isZeroValue(value)) return undefined;
  return value;
}

function compareValues(a: unknown, b: unknown): number {
  const aNum = numericText(a);
  const bNum = numericText(b);
  if (aNum !== undefined && bNum !== undefined) {
    return compareDecimalText(aNum, bNum);
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

// DynamoDB numbers span magnitudes whose most-significant-digit exponent
// (adjusted exponent) is within [-130, 125] (1E-130 .. ~9.99E+125). Exponent
// notation outside that range is rejected before any fixed-point expansion so a
// tiny hostile string such as "1e2000000000" cannot drive an unbounded alloc.
const DYNAMODB_MIN_ADJUSTED_EXPONENT = -130;
const DYNAMODB_MAX_ADJUSTED_EXPONENT = 125;
const DECIMAL_TEXT_PATTERN = /^([+-]?)(\d*)(?:\.(\d*))?(?:[eE]([+-]?\d+))?$/;
const MAX_EXACT_SIGNIFICAND = 9007199254740992n;

interface ExactDecimal {
  sign: 0 | 1 | -1;
  mantissa: bigint;
  exponent: number;
}

const ZERO_DECIMAL: ExactDecimal = { sign: 0, mantissa: 0n, exponent: 0 };

function precisionLossError(value: unknown): TheorydbError {
  return new TheorydbError(
    'ErrNumberPrecisionLoss',
    'aggregate value cannot be represented exactly: ' + String(value),
  );
}

function parseDecimalText(
  text: string,
  enforceDynamoRange: boolean,
): ExactDecimal | undefined {
  const match = DECIMAL_TEXT_PATTERN.exec(text);
  if (!match) return undefined;

  const sign = match[1] === '-' ? -1 : 1;
  const intPart = match[2] ?? '';
  const fracPart = match[3] ?? '';
  const exponentText = match[4];
  if (intPart.length === 0 && fracPart.length === 0) return undefined;

  const digits = (intPart + fracPart).replace(/^0+/, '');
  if (digits === '') return ZERO_DECIMAL;

  let baseExponent = 0;
  if (exponentText !== undefined) {
    const parsed = Number(exponentText);
    if (!Number.isSafeInteger(parsed)) {
      throw precisionLossError(text);
    }
    baseExponent = parsed;
  }

  const significant = digits.replace(/0+$/, '');
  const exponent =
    baseExponent - fracPart.length + (digits.length - significant.length);
  const adjustedExponent = exponent + significant.length - 1;
  if (
    enforceDynamoRange &&
    (adjustedExponent > DYNAMODB_MAX_ADJUSTED_EXPONENT ||
      adjustedExponent < DYNAMODB_MIN_ADJUSTED_EXPONENT)
  ) {
    throw precisionLossError(text);
  }

  return { sign, mantissa: BigInt(significant), exponent };
}

function canonicalFromDecimal(decimal: ExactDecimal): string {
  if (decimal.sign === 0) return '0';

  const digits = decimal.mantissa.toString();
  let plain: string;
  if (decimal.exponent >= 0) {
    plain = digits + '0'.repeat(decimal.exponent);
  } else {
    const pointAt = digits.length + decimal.exponent;
    if (pointAt > 0) {
      plain = digits.slice(0, pointAt) + '.' + digits.slice(pointAt);
    } else {
      plain = '0.' + '0'.repeat(-pointAt) + digits;
    }
  }
  return decimal.sign < 0 ? '-' + plain : plain;
}

function decimalFromValue(value: unknown): ExactDecimal | undefined {
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) return undefined;
    return parseDecimalText(String(value), false);
  }
  if (typeof value === 'bigint') {
    return parseDecimalText(value.toString(), false);
  }
  if (typeof value === 'string') {
    return parseDecimalText(value, true);
  }
  return undefined;
}

function numericText(value: unknown): string | undefined {
  const decimal = decimalFromValue(value);
  return decimal === undefined ? undefined : canonicalFromDecimal(decimal);
}

function decimalFraction(decimal: ExactDecimal): { num: bigint; den: bigint } {
  if (decimal.sign === 0) return { num: 0n, den: 1n };

  const magnitude = BigInt(decimal.sign) * decimal.mantissa;
  if (decimal.exponent >= 0) {
    return { num: magnitude * 10n ** BigInt(decimal.exponent), den: 1n };
  }
  return { num: magnitude, den: 10n ** BigInt(-decimal.exponent) };
}

function gcdBigInt(a: bigint, b: bigint): bigint {
  let left = a;
  let right = b;
  while (right !== 0n) {
    const remainder = left % right;
    left = right;
    right = remainder;
  }
  return left;
}

// Returns the exact IEEE-754 double equal to num/den, or undefined when no double
// represents the value exactly (a non-dyadic denominator, a significand wider
// than 53 bits, or an exponent outside the finite double range).
function fractionToNumber(num: bigint, den: bigint): number | undefined {
  if (num === 0n) return 0;

  const negative = num < 0n;
  const common = gcdBigInt(negative ? -num : num, den);
  let numerator = (negative ? -num : num) / common;
  const denominator = den / common;

  if ((denominator & (denominator - 1n)) !== 0n) return undefined;

  let exponent = 0;
  while ((numerator & 1n) === 0n) {
    numerator >>= 1n;
    exponent += 1;
  }
  let remaining = denominator;
  while (remaining > 1n) {
    remaining >>= 1n;
    exponent -= 1;
  }

  if (
    numerator >= MAX_EXACT_SIGNIFICAND ||
    exponent < -1074 ||
    exponent > 971
  ) {
    return undefined;
  }

  const value = Number(numerator) * 2 ** exponent;
  return negative ? -value : value;
}

function exactNumberFromDecimal(decimal: ExactDecimal): number | undefined {
  const { num, den } = decimalFraction(decimal);
  return fractionToNumber(num, den);
}

function requireExactNumber(decimal: ExactDecimal): number {
  const value = exactNumberFromDecimal(decimal);
  if (value === undefined) {
    throw precisionLossError(canonicalFromDecimal(decimal));
  }
  return value;
}

function toExactNumber(value: unknown): number | undefined {
  const decimal = decimalFromValue(value);
  if (decimal === undefined) return undefined;

  const exact = exactNumberFromDecimal(decimal);
  if (exact === undefined) {
    throw precisionLossError(value);
  }
  return exact;
}

class ExactAccumulator {
  private exponent: number | undefined = undefined;
  private accumulated = 0n;
  private numericCount = 0;

  add(value: unknown): void {
    const decimal = decimalFromValue(value);
    if (decimal === undefined) return;

    if (exactNumberFromDecimal(decimal) === undefined) {
      throw precisionLossError(value);
    }

    this.numericCount += 1;
    if (decimal.sign === 0) return;

    if (this.exponent === undefined || decimal.exponent < this.exponent) {
      if (this.exponent !== undefined) {
        this.accumulated *= 10n ** BigInt(this.exponent - decimal.exponent);
      }
      this.exponent = decimal.exponent;
    }
    this.accumulated +=
      BigInt(decimal.sign) *
      decimal.mantissa *
      10n ** BigInt(decimal.exponent - this.exponent);
  }

  count(): number {
    return this.numericCount;
  }

  total(): ExactDecimal {
    if (this.exponent === undefined || this.accumulated === 0n) {
      return ZERO_DECIMAL;
    }
    return {
      sign: this.accumulated < 0n ? -1 : 1,
      mantissa: this.accumulated < 0n ? -this.accumulated : this.accumulated,
      exponent: this.exponent,
    };
  }
}

function compareDecimalText(a: string, b: string): number {
  const aNeg = a.startsWith('-');
  const bNeg = b.startsWith('-');
  if (aNeg !== bNeg) return aNeg ? -1 : 1;

  const aAbs = aNeg ? a.slice(1) : a;
  const bAbs = bNeg ? b.slice(1) : b;
  const cmp = compareAbsDecimalText(aAbs, bAbs);
  return aNeg ? -cmp : cmp;
}

function compareAbsDecimalText(a: string, b: string): number {
  const [aInt = '', aFrac = ''] = a.split('.');
  const [bInt = '', bFrac = ''] = b.split('.');

  if (aInt.length !== bInt.length) return aInt.length < bInt.length ? -1 : 1;
  if (aInt !== bInt) return aInt < bInt ? -1 : 1;

  const width = Math.max(aFrac.length, bFrac.length);
  const aFracPadded = aFrac.padEnd(width, '0');
  const bFracPadded = bFrac.padEnd(width, '0');
  if (aFracPadded === bFracPadded) return 0;
  return aFracPadded < bFracPadded ? -1 : 1;
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
