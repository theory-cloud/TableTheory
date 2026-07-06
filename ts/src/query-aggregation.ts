import type { AggregateResult } from './aggregates.js';
import {
  aggregateField,
  averageField,
  countDistinct,
  GroupByQuery,
  maxField,
  minField,
  sumField,
} from './aggregates.js';

export async function sumFromAll<T extends Record<string, unknown>>(
  readAll: () => Promise<T[]>,
  field: string,
): Promise<number> {
  return sumField(await readAll(), field);
}

export async function averageFromAll<T extends Record<string, unknown>>(
  readAll: () => Promise<T[]>,
  field: string,
): Promise<number> {
  return averageField(await readAll(), field);
}

export async function minFromAll<T extends Record<string, unknown>>(
  readAll: () => Promise<T[]>,
  field: string,
): Promise<unknown> {
  return minField(await readAll(), field);
}

export async function maxFromAll<T extends Record<string, unknown>>(
  readAll: () => Promise<T[]>,
  field: string,
): Promise<unknown> {
  return maxField(await readAll(), field);
}

export async function aggregateFromAll<T extends Record<string, unknown>>(
  readAll: () => Promise<T[]>,
  field: string | undefined,
): Promise<AggregateResult> {
  return aggregateField(await readAll(), field);
}

export async function countDistinctFromAll<T extends Record<string, unknown>>(
  readAll: () => Promise<T[]>,
  field: string,
): Promise<number> {
  return countDistinct(await readAll(), field);
}

export function groupByFromAll<T extends Record<string, unknown>>(
  readAll: () => Promise<T[]>,
  field: string,
): GroupByQuery<T> {
  return new GroupByQuery(readAll, field);
}
