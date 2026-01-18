import YAML from 'yaml';

import { TheorydbError } from './errors.js';
import type { ModelSchema } from './model.js';

export interface DmsDocument {
  dms_version: string;
  namespace?: string;
  models: ModelSchema[];
}

export function parseDmsDocument(raw: string): DmsDocument {
  let parsed: unknown;
  try {
    parsed = YAML.parse(raw) as unknown;
  } catch (err) {
    throw new TheorydbError('ErrInvalidModel', 'Invalid DMS YAML/JSON', {
      cause: err,
    });
  }

  assertJsonCompatible(parsed, 'dms');
  if (!isPlainObject(parsed)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'DMS document must be an object',
    );
  }

  const doc = parsed as Partial<DmsDocument>;
  if (doc.dms_version !== '0.1') {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Unsupported dms_version: ${String(doc.dms_version ?? '')}`,
    );
  }
  if (!Array.isArray(doc.models) || doc.models.length === 0) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'DMS document must include models[]',
    );
  }

  return doc as DmsDocument;
}

export function getDmsModel(doc: DmsDocument, name: string): ModelSchema {
  for (const model of doc.models) {
    if (model?.name === name) return model;
  }
  throw new TheorydbError('ErrInvalidModel', `DMS model not found: ${name}`);
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value))
    return false;
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}

function assertJsonCompatible(value: unknown, path: string): void {
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'boolean'
  ) {
    return;
  }
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `DMS contains non-finite number at ${path}`,
      );
    }
    return;
  }
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i++) {
      assertJsonCompatible(value[i], `${path}[${i}]`);
    }
    return;
  }
  if (!isPlainObject(value)) {
    const kind =
      typeof value === 'object' && value !== null
        ? String(
            (value as { constructor?: { name?: unknown } }).constructor?.name ??
              'Object',
          )
        : typeof value;
    throw new TheorydbError(
      'ErrInvalidModel',
      `DMS contains non-JSON value at ${path} (${kind})`,
    );
  }

  for (const [k, v] of Object.entries(value)) {
    assertJsonCompatible(v, `${path}.${k}`);
  }
}
