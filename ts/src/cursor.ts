import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { fromDynamoJson, toDynamoJson } from './dynamo-json.js';
import { TheorydbError } from './errors.js';

export type CursorSort = 'ASC' | 'DESC';

export interface Cursor {
  lastKey: Record<string, AttributeValue>;
  index?: string;
  sort?: CursorSort;
}

export function encodeCursor(cursor: Cursor): string {
  if (!cursor?.lastKey || typeof cursor.lastKey !== 'object') {
    throw new TheorydbError('ErrInvalidModel', 'Cursor lastKey is required');
  }

  const lastKeyJson: Record<string, unknown> = {};
  for (const key of Object.keys(cursor.lastKey).sort()) {
    const av = cursor.lastKey[key];
    if (!av)
      throw new TheorydbError(
        'ErrInvalidModel',
        `Cursor lastKey missing value: ${key}`,
      );
    lastKeyJson[key] = toDynamoJson(av);
  }

  const parts: string[] = [];
  parts.push(`"lastKey":${stableStringify(lastKeyJson)}`);
  if (cursor.index) parts.push(`"index":${JSON.stringify(cursor.index)}`);
  if (cursor.sort) parts.push(`"sort":${JSON.stringify(cursor.sort)}`);

  const json = `{${parts.join(',')}}`;
  return base64UrlEncode(Buffer.from(json, 'utf8'));
}

export function decodeCursor(encoded: string): Cursor {
  const raw = String(encoded ?? '').trim();
  if (!raw)
    throw new TheorydbError('ErrInvalidModel', 'Cursor string is empty');

  const jsonStr = base64UrlDecode(raw).toString('utf8');
  const parsed = JSON.parse(jsonStr) as {
    lastKey?: unknown;
    index?: unknown;
    sort?: unknown;
  };

  if (!parsed || typeof parsed !== 'object')
    throw new TheorydbError('ErrInvalidModel', 'Cursor JSON is invalid');

  const lastKeyObj = parsed.lastKey;
  if (
    !lastKeyObj ||
    typeof lastKeyObj !== 'object' ||
    Array.isArray(lastKeyObj)
  ) {
    throw new TheorydbError('ErrInvalidModel', 'Cursor lastKey is invalid');
  }

  const lastKey: Record<string, AttributeValue> = {};
  for (const key of Object.keys(lastKeyObj as Record<string, unknown>)) {
    lastKey[key] = fromDynamoJson((lastKeyObj as Record<string, unknown>)[key]);
  }

  const out: Cursor = { lastKey };
  if (typeof parsed.index === 'string') out.index = parsed.index;
  if (parsed.sort === 'ASC' || parsed.sort === 'DESC') out.sort = parsed.sort;

  return out;
}

function base64UrlEncode(buf: Buffer): string {
  return buf.toString('base64').replace(/\+/g, '-').replace(/\//g, '_');
}

function base64UrlDecode(input: string): Buffer {
  const b64 = input.replace(/-/g, '+').replace(/_/g, '/');
  return Buffer.from(b64, 'base64');
}

function stableStringify(value: unknown): string {
  if (value === undefined) return 'null';
  if (value === null) return 'null';
  if (typeof value === 'string') return JSON.stringify(value);
  if (typeof value === 'number') return JSON.stringify(value);
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`;

  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    const keys = Object.keys(obj)
      .filter((k) => obj[k] !== undefined)
      .sort();
    const parts = keys.map(
      (k) => `${JSON.stringify(k)}:${stableStringify(obj[k])}`,
    );
    return `{${parts.join(',')}}`;
  }

  return JSON.stringify(value);
}
