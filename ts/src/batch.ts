import type { AttributeValue, WriteRequest } from '@aws-sdk/client-dynamodb';

export interface RetryOptions {
  maxAttempts?: number;
  baseDelayMs?: number;
}

export interface BatchGetResult<T = Record<string, unknown>> {
  items: T[];
  unprocessedKeys: Array<Record<string, AttributeValue>>;
}

export interface BatchWriteResult {
  unprocessed: WriteRequest[];
}

export function chunk<T>(items: T[], size: number): T[][] {
  if (size <= 0) throw new Error('chunk size must be > 0');
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) {
    out.push(items.slice(i, i + size));
  }
  return out;
}

export async function sleep(ms: number): Promise<void> {
  await new Promise((r) => setTimeout(r, ms));
}
