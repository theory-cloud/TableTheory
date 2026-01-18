import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';

export function encodeEncryptedPayload(av: AttributeValue): Uint8Array {
  return Buffer.from(encodeAV(av), 'utf8');
}

export function decodeEncryptedPayload(bytes: Uint8Array): AttributeValue {
  let parsed: unknown;
  try {
    parsed = JSON.parse(Buffer.from(bytes).toString('utf8'));
  } catch (err) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Failed to decode encrypted payload',
      { cause: err },
    );
  }

  return decodeAV(parsed);
}

function encodeAV(av: AttributeValue): string {
  if ('S' in av && av.S !== undefined)
    return `{"t":"S","s":${JSON.stringify(av.S)}}`;
  if ('N' in av && av.N !== undefined)
    return `{"t":"N","n":${JSON.stringify(av.N)}}`;
  if ('B' in av && av.B !== undefined)
    return `{"t":"B","b":${JSON.stringify(
      Buffer.from(av.B).toString('base64'),
    )}}`;
  if ('BOOL' in av && av.BOOL !== undefined)
    return `{"t":"BOOL","bool":${av.BOOL ? 'true' : 'false'}}`;
  if ('NULL' in av && av.NULL !== undefined)
    return `{"t":"NULL","null":${av.NULL ? 'true' : 'false'}}`;
  if ('SS' in av && av.SS !== undefined)
    return `{"t":"SS","ss":${JSON.stringify(av.SS)}}`;
  if ('NS' in av && av.NS !== undefined)
    return `{"t":"NS","ns":${JSON.stringify(av.NS)}}`;
  if ('BS' in av && av.BS !== undefined)
    return `{"t":"BS","bs":${JSON.stringify(
      av.BS.map((b) => Buffer.from(b).toString('base64')),
    )}}`;

  if ('L' in av && av.L !== undefined) {
    const parts = av.L.map(encodeAV).join(',');
    return `{"t":"L","l":[${parts}]}`;
  }

  if ('M' in av && av.M !== undefined) {
    const keys = Object.keys(av.M).sort();
    const parts: string[] = [];
    for (const key of keys) {
      const child = av.M[key];
      if (!child)
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          `Invalid map value: ${key}`,
        );
      parts.push(`${JSON.stringify(key)}:${encodeAV(child)}`);
    }
    return `{"t":"M","m":{${parts.join(',')}}}`;
  }

  throw new TheorydbError(
    'ErrInvalidEncryptedEnvelope',
    'Unsupported encrypted payload type',
  );
}

function decodeAV(input: unknown): AttributeValue {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Encrypted payload must be an object',
    );
  }

  const obj = input as Record<string, unknown>;
  const t = obj.t;
  if (typeof t !== 'string') {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Encrypted payload missing t',
    );
  }

  switch (t) {
    case 'S': {
      const s = obj.s;
      if (typeof s !== 'string')
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload S must be a string',
        );
      return { S: s };
    }
    case 'N': {
      const n = obj.n;
      if (typeof n !== 'string')
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload N must be a string',
        );
      return { N: n };
    }
    case 'B': {
      const b = obj.b;
      if (typeof b !== 'string')
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload B must be base64 string',
        );
      return { B: Buffer.from(b, 'base64') };
    }
    case 'BOOL': {
      const v = obj.bool;
      if (typeof v !== 'boolean')
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload BOOL must be boolean',
        );
      return { BOOL: v };
    }
    case 'NULL': {
      const v = obj.null;
      if (v !== true)
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload NULL must be true',
        );
      return { NULL: true };
    }
    case 'SS': {
      const v = obj.ss;
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string'))
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload SS must be list of strings',
        );
      return { SS: v as string[] };
    }
    case 'NS': {
      const v = obj.ns;
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string'))
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload NS must be list of strings',
        );
      return { NS: v as string[] };
    }
    case 'BS': {
      const v = obj.bs;
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string'))
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload BS must be list of base64 strings',
        );
      return { BS: (v as string[]).map((s) => Buffer.from(s, 'base64')) };
    }
    case 'L': {
      const v = obj.l;
      if (!Array.isArray(v))
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload L must be list',
        );
      return { L: v.map(decodeAV) };
    }
    case 'M': {
      const v = obj.m;
      if (!v || typeof v !== 'object' || Array.isArray(v))
        throw new TheorydbError(
          'ErrInvalidEncryptedEnvelope',
          'Encrypted payload M must be map',
        );
      const out: Record<string, AttributeValue> = {};
      for (const key of Object.keys(v as Record<string, unknown>)) {
        out[key] = decodeAV((v as Record<string, unknown>)[key]);
      }
      return { M: out };
    }
    default:
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        `Unsupported encrypted payload type: ${t}`,
      );
  }
}
