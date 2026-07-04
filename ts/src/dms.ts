import YAML from 'yaml';

import { TheorydbError } from './errors.js';
import {
  defineModel,
  type AttributeSchema,
  type IndexSchema,
  type KeySchema,
  type Model,
  type ModelSchema,
} from './model.js';

export interface DmsDocument {
  dms_version: string;
  namespace?: string;
  models: ModelSchema[];
}

export interface DmsCompareOptions {
  ignoreTableName?: boolean;
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
  for (const model of doc.models) {
    validateDmsNamingConvention(model);
    defineModel(model);
  }

  return doc as DmsDocument;
}

export function getDmsModel(doc: DmsDocument, name: string): ModelSchema {
  for (const model of doc.models) {
    if (model?.name === name) return model;
  }
  throw new TheorydbError('ErrInvalidModel', `DMS model not found: ${name}`);
}

export function modelToDmsModel(
  model: Model | ModelSchema,
): Readonly<ModelSchema> {
  return normalizeSchema(extractSchema(model), {});
}

export function assertModelsEquivalent(
  got: Model | ModelSchema,
  want: ModelSchema,
  options: DmsCompareOptions = {},
): void {
  const normalizedGot = normalizeSchema(extractSchema(got), options);
  const normalizedWant = normalizeSchema(want, options);
  if (stableJson(normalizedGot) === stableJson(normalizedWant)) return;

  throw new TheorydbError(
    'ErrInvalidModel',
    `models not equivalent\nwant=${stableJson(normalizedWant)}\ngot=${stableJson(
      normalizedGot,
    )}`,
  );
}

function extractSchema(model: Model | ModelSchema): ModelSchema {
  if ('schema' in model) return model.schema;
  return model;
}

function normalizeSchema(
  schema: ModelSchema,
  options: DmsCompareOptions,
): ModelSchema {
  const normalized: ModelSchema = {
    name: schema.name,
    table: { name: options.ignoreTableName ? '' : schema.table.name },
    naming: {
      convention: schema.naming?.convention ?? 'camelCase',
    },
    keys: normalizeKeys(schema.keys),
    write_policy: {
      mode: schema.write_policy?.mode ?? 'mutable',
      protected_attributes: [
        ...new Set(schema.write_policy?.protected_attributes ?? []),
      ].sort(),
    },
    attributes: [...schema.attributes]
      .map(normalizeAttribute)
      .sort((a, b) => a.attribute.localeCompare(b.attribute)),
    indexes: [...(schema.indexes ?? [])]
      .map(normalizeIndex)
      .sort((a, b) => a.name.localeCompare(b.name)),
  };
  return normalized;
}

function normalizeKeys(keys: ModelSchema['keys']): ModelSchema['keys'] {
  const normalized: ModelSchema['keys'] = {
    partition: normalizeKey(keys.partition),
  };
  if (keys.sort) normalized.sort = normalizeKey(keys.sort);
  return normalized;
}

function normalizeKey(key: KeySchema): KeySchema {
  return { attribute: key.attribute, type: key.type };
}

function normalizeAttribute(attr: AttributeSchema): AttributeSchema {
  const roles = [...(attr.roles ?? [])]
    .filter((role) => role.length > 0)
    .sort();
  const normalized: AttributeSchema = {
    attribute: attr.attribute,
    type: attr.type,
    required: attr.required === true,
    optional: attr.optional === true,
    omit_empty: attr.omit_empty === true,
    roles,
    json: attr.json === true,
    binary: attr.binary === true,
  };
  if (attr.encryption !== undefined && attr.encryption !== null) {
    normalized.encryption = { v: 1 };
  }
  return normalized;
}

function normalizeIndex(idx: IndexSchema): IndexSchema {
  const normalized: IndexSchema = {
    name: idx.name,
    type: idx.type,
    partition: normalizeKey(idx.partition),
    projection: normalizeProjection(idx.projection),
  };
  if (idx.sort) normalized.sort = normalizeKey(idx.sort);
  return normalized;
}

function normalizeProjection(
  projection: IndexSchema['projection'],
): NonNullable<IndexSchema['projection']> {
  return {
    type: projection?.type ?? 'ALL',
    fields: [...(projection?.fields ?? [])].sort(),
  };
}

function stableJson(value: unknown): string {
  return JSON.stringify(sortJsonValue(value));
}

function sortJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortJsonValue);
  if (!isPlainObject(value)) return value;
  const out: Record<string, unknown> = {};
  for (const key of Object.keys(value).sort()) {
    const child = value[key];
    if (child === undefined) continue;
    out[key] = sortJsonValue(child);
  }
  return out;
}

function validateDmsNamingConvention(model: unknown): void {
  if (!isPlainObject(model)) {
    throw new TheorydbError('ErrInvalidModel', 'DMS model must be an object');
  }
  const name = typeof model.name === 'string' ? model.name : '<unknown>';
  const naming = model.naming;
  if (naming === undefined) return;
  if (!isPlainObject(naming)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `DMS model ${name}: naming must be an object`,
    );
  }
  const convention = naming.convention;
  if (
    convention === undefined ||
    convention === 'camelCase' ||
    convention === 'snake_case' ||
    convention === 'dynamorm'
  ) {
    return;
  }
  throw new TheorydbError(
    'ErrInvalidModel',
    `DMS model ${name}: unsupported naming.convention ${String(convention)}`,
  );
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
      typeof value === 'object'
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
