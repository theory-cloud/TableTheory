import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import type { AttributeSchema, Model } from './model.js';
import { TheorydbError } from './errors.js';

export function nowRfc3339Nano(date = new Date()): string {
  const iso = date.toISOString(); // always has milliseconds: YYYY-MM-DDTHH:mm:ss.sssZ
  return iso.replace(/\.(\d{3})Z$/, '.$1000000Z');
}

export function isEmpty(value: unknown): boolean {
  if (value === null || value === undefined) return true;

  if (typeof value === 'string') return value.length === 0;
  if (typeof value === 'number') return value === 0;
  if (typeof value === 'boolean') return value === false;

  if (value instanceof Date) return Number.isNaN(value.getTime());

  if (Array.isArray(value)) return value.length === 0;

  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) return true;
    return entries.every(([, v]) => isEmpty(v));
  }

  return false;
}

export function marshalKey(
  model: Model,
  key: Record<string, unknown>,
): Record<string, AttributeValue> {
  const pkName = model.roles.pk;
  const pkSchema = model.attributes.get(pkName);
  if (!pkSchema)
    throw new TheorydbError(
      'ErrInvalidModel',
      `Model missing pk attribute schema: ${pkName}`,
    );

  const out: Record<string, AttributeValue> = {};

  const pkValue = key[pkName];
  if (isEmpty(pkValue))
    throw new TheorydbError(
      'ErrMissingPrimaryKey',
      `Missing partition key: ${pkName}`,
    );
  out[pkName] = marshalScalar(pkSchema, pkValue);

  if (model.roles.sk) {
    const skName = model.roles.sk;
    const skSchema = model.attributes.get(skName);
    if (!skSchema)
      throw new TheorydbError(
        'ErrInvalidModel',
        `Model missing sk attribute schema: ${skName}`,
      );
    const skValue = key[skName];
    if (isEmpty(skValue))
      throw new TheorydbError(
        'ErrMissingPrimaryKey',
        `Missing sort key: ${skName}`,
      );
    out[skName] = marshalScalar(skSchema, skValue);
  }

  return out;
}

export function marshalPutItem(
  model: Model,
  item: Record<string, unknown>,
  opts: { now?: string } = {},
): Record<string, AttributeValue> {
  const now = opts.now ?? nowRfc3339Nano();

  const knownAttributes = new Set(
    model.schema.attributes.map((a) => a.attribute),
  );
  for (const key of Object.keys(item)) {
    if (!knownAttributes.has(key)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Unknown attribute for model ${model.name}: ${key}`,
      );
    }
  }

  const out: Record<string, AttributeValue> = {};

  // Enforce keys exist.
  Object.assign(out, marshalKey(model, item));

  for (const attr of model.schema.attributes) {
    const name = attr.attribute;
    if (name === model.roles.pk || name === model.roles.sk) continue;

    // Lifecycle fields.
    if (name === model.roles.createdAt) {
      out[name] = { S: now };
      continue;
    }
    if (name === model.roles.updatedAt) {
      out[name] = { S: now };
      continue;
    }
    if (name === model.roles.version) {
      const v = item[name];
      out[name] = { N: String(isEmpty(v) ? 0 : (v as number)) };
      continue;
    }

    const value = item[name];
    if (value === undefined) continue;

    if (attr.omit_empty && isEmpty(value)) continue;

    out[name] = marshalScalar(attr, value);
  }

  return out;
}

export function unmarshalItem(
  model: Model,
  item: Record<string, AttributeValue>,
): Record<string, unknown> {
  const out: Record<string, unknown> = {};

  for (const [name, av] of Object.entries(item)) {
    const schema = model.attributes.get(name);
    if (!schema) {
      out[name] = av;
      continue;
    }
    out[name] = unmarshalScalar(schema, av);
  }

  return out;
}

export function marshalScalar(
  schema: Readonly<AttributeSchema>,
  value: unknown,
): AttributeValue {
  const converted =
    schema.converter !== undefined
      ? schema.converter.toDynamoValue(value)
      : value;

  if (schema.json) {
    if (schema.type !== 'S') {
      throw new TheorydbError(
        'ErrInvalidModel',
        `json attributes must be type S: ${schema.attribute}`,
      );
    }
    if (converted === undefined) {
      throw new TheorydbError(
        'ErrInvalidModel',
        'Undefined values are not supported',
      );
    }
    if (converted === null) return { NULL: true };
    return { S: stableJsonStringify(converted, schema.attribute) };
  }

  switch (schema.type) {
    case 'S':
      if (typeof converted !== 'string')
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected string for ${schema.attribute}`,
        );
      return { S: converted };
    case 'N':
      if (typeof converted === 'number') return { N: String(converted) };
      if (typeof converted === 'bigint') return { N: converted.toString() };
      if (typeof converted === 'string') return { N: converted };
      throw new TheorydbError(
        'ErrInvalidModel',
        `Expected number for ${schema.attribute}`,
      );
    case 'B': {
      if (converted instanceof Uint8Array) return { B: converted };
      throw new TheorydbError(
        'ErrInvalidModel',
        `Expected Uint8Array for ${schema.attribute}`,
      );
    }
    case 'SS': {
      if (!Array.isArray(converted)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected string[] for ${schema.attribute}`,
        );
      }
      const ss = converted.map((v) => {
        if (typeof v !== 'string') {
          throw new TheorydbError(
            'ErrInvalidModel',
            `Expected string[] for ${schema.attribute}`,
          );
        }
        return v;
      });
      if (ss.length === 0) return { NULL: true };
      return { SS: ss };
    }
    case 'NS': {
      if (!Array.isArray(converted)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected number[] for ${schema.attribute}`,
        );
      }
      const ns = converted.map((v) => {
        if (typeof v === 'number') return String(v);
        if (typeof v === 'bigint') return v.toString();
        if (typeof v === 'string') return v;
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected number[] for ${schema.attribute}`,
        );
      });
      if (ns.length === 0) return { NULL: true };
      return { NS: ns };
    }
    case 'BS': {
      if (!Array.isArray(converted)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected Uint8Array[] for ${schema.attribute}`,
        );
      }
      const bs = converted.map((v) => {
        if (v instanceof Uint8Array) return v;
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected Uint8Array[] for ${schema.attribute}`,
        );
      });
      if (bs.length === 0) return { NULL: true };
      return { BS: bs };
    }
    case 'BOOL':
      if (typeof converted !== 'boolean')
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected boolean for ${schema.attribute}`,
        );
      return { BOOL: converted };
    case 'NULL':
      return { NULL: true };
    case 'L': {
      if (!Array.isArray(converted)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected array for ${schema.attribute}`,
        );
      }
      return { L: converted.map(marshalDocumentValue) };
    }
    case 'M': {
      if (
        converted === null ||
        typeof converted !== 'object' ||
        Array.isArray(converted)
      ) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Expected object for ${schema.attribute}`,
        );
      }
      const out: Record<string, AttributeValue> = {};
      for (const [k, v] of Object.entries(
        converted as Record<string, unknown>,
      )) {
        out[k] = marshalDocumentValue(v);
      }
      return { M: out };
    }
    default:
      throw new TheorydbError(
        'ErrInvalidModel',
        `Unsupported type ${schema.type} for ${schema.attribute}`,
      );
  }
}

export function unmarshalScalar(
  schema: Readonly<AttributeSchema>,
  av: AttributeValue,
): unknown {
  const applyConverter = (value: unknown): unknown =>
    schema.converter !== undefined
      ? schema.converter.fromDynamoValue(value)
      : value;

  if (schema.json) {
    if (schema.type !== 'S') {
      throw new TheorydbError(
        'ErrInvalidModel',
        `json attributes must be type S: ${schema.attribute}`,
      );
    }

    if ('NULL' in av && av.NULL) return applyConverter(null);

    if ('S' in av && av.S !== undefined) {
      const raw = av.S;
      try {
        return applyConverter(JSON.parse(raw));
      } catch (err) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Invalid JSON value for ${schema.attribute}`,
          { cause: err },
        );
      }
    }

    throw new TheorydbError(
      'ErrInvalidModel',
      `Unsupported AttributeValue for ${schema.attribute}`,
    );
  }

  switch (schema.type) {
    case 'S':
      if ('S' in av && av.S !== undefined) return applyConverter(av.S);
      break;
    case 'N':
      if ('N' in av && av.N !== undefined) return applyConverter(Number(av.N));
      break;
    case 'B':
      if ('B' in av && av.B !== undefined)
        return applyConverter(Buffer.from(av.B));
      break;
    case 'SS':
      if ('SS' in av && av.SS !== undefined)
        return applyConverter(av.SS.slice());
      if ('NULL' in av && av.NULL) return applyConverter([]);
      break;
    case 'NS':
      if ('NS' in av && av.NS !== undefined)
        return applyConverter(av.NS.map((n) => Number(n)));
      if ('NULL' in av && av.NULL) return applyConverter([]);
      break;
    case 'BS':
      if ('BS' in av && av.BS !== undefined)
        return applyConverter(av.BS.map((b) => Buffer.from(b)));
      if ('NULL' in av && av.NULL) return applyConverter([]);
      break;
    case 'BOOL':
      if ('BOOL' in av && av.BOOL !== undefined) return applyConverter(av.BOOL);
      break;
    case 'NULL':
      if ('NULL' in av && av.NULL) return applyConverter(null);
      break;
    case 'L':
      if ('L' in av && av.L !== undefined)
        return applyConverter(av.L.map(unmarshalDocumentValue));
      break;
    case 'M':
      if ('M' in av && av.M !== undefined) {
        const out: Record<string, unknown> = {};
        for (const [k, v] of Object.entries(av.M)) {
          if (!v)
            throw new TheorydbError(
              'ErrInvalidModel',
              `Invalid map value for ${schema.attribute}`,
            );
          out[k] = unmarshalDocumentValue(v);
        }
        return applyConverter(out);
      }
      break;
    default:
      throw new TheorydbError(
        'ErrInvalidModel',
        `Unsupported type ${schema.type} for ${schema.attribute}`,
      );
  }

  throw new TheorydbError(
    'ErrInvalidModel',
    `Unsupported AttributeValue for ${schema.attribute}`,
  );
}

function stableJsonStringify(value: unknown, label: string): string {
  const normalized = normalizeJsonValue(value, label);
  return JSON.stringify(normalized);
}

function normalizeJsonValue(value: unknown, path: string): unknown {
  if (value === undefined) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Undefined values are not supported',
    );
  }
  if (value === null) return null;

  if (typeof value === 'string' || typeof value === 'boolean') return value;
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Non-finite number in JSON value at ${path}`,
      );
    }
    return value;
  }

  if (Array.isArray(value)) {
    return value.map((v, idx) => normalizeJsonValue(v, `${path}[${idx}]`));
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
      `Non-JSON value at ${path} (${kind})`,
    );
  }

  const out: Record<string, unknown> = {};
  const keys = Object.keys(value).sort();
  for (const key of keys) {
    out[key] = normalizeJsonValue(
      (value as Record<string, unknown>)[key],
      `${path}.${key}`,
    );
  }

  return out;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (value === null) return false;
  if (typeof value !== 'object') return false;
  if (Array.isArray(value)) return false;
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}

export function marshalDocumentValue(value: unknown): AttributeValue {
  if (value === undefined) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Undefined values are not supported',
    );
  }
  if (value === null) return { NULL: true };

  if (typeof value === 'string') return { S: value };
  if (typeof value === 'number') return { N: String(value) };
  if (typeof value === 'bigint') return { N: value.toString() };
  if (typeof value === 'boolean') return { BOOL: value };

  if (value instanceof Uint8Array) return { B: value };

  if (Array.isArray(value)) return { L: value.map(marshalDocumentValue) };

  if (typeof value === 'object') {
    const out: Record<string, AttributeValue> = {};
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      out[k] = marshalDocumentValue(v);
    }
    return { M: out };
  }

  throw new TheorydbError(
    'ErrInvalidModel',
    `Unsupported document value: ${typeof value}`,
  );
}

export function unmarshalDocumentValue(av: AttributeValue): unknown {
  if ('S' in av && av.S !== undefined) return av.S;
  if ('N' in av && av.N !== undefined) return Number(av.N);
  if ('B' in av && av.B !== undefined) return Buffer.from(av.B);
  if ('SS' in av && av.SS !== undefined) return av.SS.slice();
  if ('NS' in av && av.NS !== undefined) return av.NS.map((n) => Number(n));
  if ('BS' in av && av.BS !== undefined)
    return av.BS.map((b) => Buffer.from(b));
  if ('BOOL' in av && av.BOOL !== undefined) return av.BOOL;
  if ('NULL' in av && av.NULL) return null;

  if ('L' in av && av.L !== undefined) return av.L.map(unmarshalDocumentValue);

  if ('M' in av && av.M !== undefined) {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(av.M)) {
      if (!v)
        throw new TheorydbError('ErrInvalidModel', `Invalid map value: ${k}`);
      out[k] = unmarshalDocumentValue(v);
    }
    return out;
  }

  throw new TheorydbError('ErrInvalidModel', 'Unsupported AttributeValue');
}
