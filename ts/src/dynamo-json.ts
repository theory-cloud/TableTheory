import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';

export function toDynamoJson(av: AttributeValue): unknown {
  if ('S' in av && av.S !== undefined) return { S: av.S };
  if ('N' in av && av.N !== undefined) return { N: av.N };
  if ('BOOL' in av && av.BOOL !== undefined) return { BOOL: av.BOOL };
  if ('NULL' in av && av.NULL !== undefined) return { NULL: av.NULL };
  if ('SS' in av && av.SS !== undefined) return { SS: av.SS };
  if ('NS' in av && av.NS !== undefined) return { NS: av.NS };
  if ('B' in av && av.B !== undefined)
    return { B: Buffer.from(av.B).toString('base64') };
  if ('BS' in av && av.BS !== undefined)
    return { BS: av.BS.map((b) => Buffer.from(b).toString('base64')) };
  if ('L' in av && av.L !== undefined) return { L: av.L.map(toDynamoJson) };
  if ('M' in av && av.M !== undefined) {
    const out: Record<string, unknown> = {};
    for (const key of Object.keys(av.M)) {
      const child = av.M[key];
      if (!child)
        throw new TheorydbError('ErrInvalidModel', `Invalid map value: ${key}`);
      out[key] = toDynamoJson(child);
    }
    return { M: out };
  }

  throw new TheorydbError('ErrInvalidModel', 'Invalid AttributeValue JSON');
}

export function fromDynamoJson(input: unknown): AttributeValue {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    throw new TheorydbError('ErrInvalidModel', 'Invalid AttributeValue JSON');
  }

  const keys = Object.keys(input as Record<string, unknown>);
  if (keys.length !== 1)
    throw new TheorydbError('ErrInvalidModel', 'Invalid AttributeValue JSON');
  const k = keys[0]!;
  const v = (input as Record<string, unknown>)[k];

  switch (k) {
    case 'S':
      if (typeof v !== 'string')
        throw new TheorydbError('ErrInvalidModel', 'Invalid S value');
      return { S: v };
    case 'N':
      if (typeof v !== 'string')
        throw new TheorydbError('ErrInvalidModel', 'Invalid N value');
      return { N: v };
    case 'BOOL':
      if (typeof v !== 'boolean')
        throw new TheorydbError('ErrInvalidModel', 'Invalid BOOL value');
      return { BOOL: v };
    case 'NULL':
      if (v !== true)
        throw new TheorydbError('ErrInvalidModel', 'Invalid NULL value');
      return { NULL: true };
    case 'SS':
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string')) {
        throw new TheorydbError('ErrInvalidModel', 'Invalid SS value');
      }
      return { SS: v as string[] };
    case 'NS':
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string')) {
        throw new TheorydbError('ErrInvalidModel', 'Invalid NS value');
      }
      return { NS: v as string[] };
    case 'B':
      if (typeof v !== 'string')
        throw new TheorydbError('ErrInvalidModel', 'Invalid B value');
      return { B: Buffer.from(v, 'base64') };
    case 'BS':
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string')) {
        throw new TheorydbError('ErrInvalidModel', 'Invalid BS value');
      }
      return { BS: (v as string[]).map((s) => Buffer.from(s, 'base64')) };
    case 'L':
      if (!Array.isArray(v))
        throw new TheorydbError('ErrInvalidModel', 'Invalid L value');
      return { L: (v as unknown[]).map(fromDynamoJson) };
    case 'M': {
      if (!v || typeof v !== 'object' || Array.isArray(v)) {
        throw new TheorydbError('ErrInvalidModel', 'Invalid M value');
      }
      const out: Record<string, AttributeValue> = {};
      for (const key of Object.keys(v as Record<string, unknown>)) {
        out[key] = fromDynamoJson((v as Record<string, unknown>)[key]);
      }
      return { M: out };
    }
    default:
      throw new TheorydbError(
        'ErrInvalidModel',
        `Unsupported AttributeValue type: ${k}`,
      );
  }
}
