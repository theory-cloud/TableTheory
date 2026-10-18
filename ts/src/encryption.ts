import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';
import type { AttributeSchema, Model } from './model.js';
import {
  isEmpty,
  isEmptyAttribute,
  marshalKey,
  marshalScalar,
  nowRfc3339Nano,
} from './marshal.js';
import {
  decodeEncryptedPayload,
  encodeEncryptedPayload,
} from './encryption-avjson.js';

/**
 * Identifies the stored record an encrypted envelope belongs to. Envelope v2
 * authenticates this identity in the AES-GCM additional data so an envelope
 * copied onto another record or table cannot be decrypted.
 */
export interface RecordBinding {
  table: string;
  partitionKey: AttributeValue;
  sortKey?: AttributeValue;
}

export interface EncryptionContext {
  model: string;
  attribute: string;
  binding?: RecordBinding;
}

export interface EncryptedEnvelope {
  v: 1 | 2;
  edk: Uint8Array;
  nonce: Uint8Array;
  ct: Uint8Array;
}

export interface EncryptionProvider {
  encrypt(
    plaintext: Uint8Array,
    ctx: EncryptionContext,
  ): Promise<EncryptedEnvelope>;
  decrypt(
    envelope: EncryptedEnvelope,
    ctx: EncryptionContext,
  ): Promise<Uint8Array>;
}

function bindingIsValid(binding: RecordBinding): boolean {
  return binding.table.trim() !== '' && Boolean(binding.partitionKey);
}

/**
 * Canonical key encoding shared by every runtime: `S:<value>` for strings,
 * `N:<value>` for numbers, and `B:<standard base64>` for binary.
 */
export function canonicalKeyValue(av: AttributeValue): string {
  if ('S' in av && typeof av.S === 'string') return `S:${av.S}`;
  if ('N' in av && typeof av.N === 'string') return `N:${av.N}`;
  if ('B' in av && av.B) return `B:${Buffer.from(av.B).toString('base64')}`;
  return '';
}

/**
 * The AES-GCM additional authenticated data for an envelope version. v1 binds
 * only the attribute name (legacy); v2 binds the record identity as well and
 * therefore requires a binding.
 */
export function aadForContext(
  ctx: EncryptionContext,
  version: 1 | 2,
): Uint8Array {
  if (version === 1) {
    return new TextEncoder().encode(
      `theorydb:encrypted:v1|attr=${ctx.attribute}`,
    );
  }

  const binding = ctx.binding;
  if (!binding || !bindingIsValid(binding)) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'encrypted envelope v2 requires the record binding',
    );
  }

  let aad =
    `theorydb:encrypted:v2|attr=${ctx.attribute}` +
    `|table=${binding.table.trim()}` +
    `|pk=${canonicalKeyValue(binding.partitionKey)}`;
  if (binding.sortKey !== undefined) {
    aad += `|sk=${canonicalKeyValue(binding.sortKey)}`;
  }
  return new TextEncoder().encode(aad);
}

/** Writes use v2 whenever the record identity is known. */
export function envelopeVersionForContext(ctx: EncryptionContext): 1 | 2 {
  if (ctx.binding === undefined) return 1;
  if (!bindingIsValid(ctx.binding)) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'encrypted envelope record binding requires a table name and partition key',
    );
  }
  return 2;
}

/** Builds the record binding from an already-marshaled model key. */
export function recordBindingFromKey(
  model: Model,
  key: Record<string, AttributeValue>,
): RecordBinding {
  const partitionKey = key[model.roles.pk];
  if (!partitionKey) {
    throw new TheorydbError(
      'ErrMissingPrimaryKey',
      `Missing partition key: ${model.roles.pk}`,
    );
  }

  const skName = model.roles.sk;
  if (skName === undefined) {
    return { table: model.tableName, partitionKey };
  }

  const sortKey = key[skName];
  if (!sortKey) {
    throw new TheorydbError(
      'ErrMissingPrimaryKey',
      `Missing sort key: ${skName}`,
    );
  }
  return { table: model.tableName, partitionKey, sortKey };
}

/**
 * Derives a binding from a full or projected item. Returns undefined when the
 * item does not carry its key attributes, so v2 fails closed while legacy v1
 * envelopes still decrypt.
 */
export function recordBindingFromItem(
  model: Model,
  item: Record<string, AttributeValue>,
): RecordBinding | undefined {
  const partitionKey = item[model.roles.pk];
  if (!partitionKey) return undefined;

  const skName = model.roles.sk;
  if (skName === undefined) {
    return { table: model.tableName, partitionKey };
  }

  const sortKey = item[skName];
  if (!sortKey) return undefined;
  return { table: model.tableName, partitionKey, sortKey };
}

export function envelopeToAttributeValue(
  env: EncryptedEnvelope,
): AttributeValue {
  return {
    M: {
      v: { N: String(env.v) },
      edk: { B: env.edk },
      nonce: { B: env.nonce },
      ct: { B: env.ct },
    },
  };
}

export function modelHasEncryptedAttributes(model: Model): boolean {
  return model.schema.attributes.some((a) => a.encryption !== undefined);
}

export async function marshalPutItemEncrypted(
  model: Model,
  item: Record<string, unknown>,
  provider: EncryptionProvider,
  opts: { now?: string } = {},
): Promise<Record<string, AttributeValue>> {
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
  const key = marshalKey(model, item);
  Object.assign(out, key);
  const binding = recordBindingFromKey(model, key);

  for (const attr of model.schema.attributes) {
    const name = attr.attribute;
    if (name === model.roles.pk || name === model.roles.sk) continue;

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
      out[name] = { N: String(isEmpty(v) ? 0 : v) };
      continue;
    }

    const value = item[name];
    if (value === undefined) continue;
    if (attr.omit_empty && isEmptyAttribute(attr, value)) continue;

    if (attr.encryption !== undefined) {
      out[name] = await encryptAttributeValue(attr, value, provider, {
        model: model.name,
        attribute: name,
        binding,
      });
      continue;
    }

    out[name] = marshalScalar(attr, value);
  }

  return out;
}

export async function decryptItemAttributes(
  model: Model,
  item: Record<string, AttributeValue>,
  provider: EncryptionProvider,
  // Callers that already know the record key (an update returning only the
  // changed attributes) can supply the binding their item projection omits.
  knownBinding?: RecordBinding,
): Promise<Record<string, AttributeValue>> {
  const out: Record<string, AttributeValue> = { ...item };
  const binding = knownBinding ?? recordBindingFromItem(model, item);

  for (const attr of model.schema.attributes) {
    if (attr.encryption === undefined) continue;

    const name = attr.attribute;
    const av = out[name];
    if (!av) continue;

    const env = parseEnvelope(av);
    const ctx: EncryptionContext = {
      model: model.name,
      attribute: name,
    };
    if (binding !== undefined) ctx.binding = binding;

    const plaintext = await provider.decrypt(env, ctx);
    out[name] = plaintextAttributeValue(attr, plaintext);
  }

  return out;
}

export async function encryptAttributeValue(
  schema: Readonly<AttributeSchema>,
  value: unknown,
  provider: EncryptionProvider,
  ctx: EncryptionContext,
): Promise<AttributeValue> {
  const bytes = toBytes(schema, value);
  const expectedVersion = envelopeVersionForContext(ctx);
  const env = await provider.encrypt(bytes, ctx);
  if (env.v !== expectedVersion)
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      `Encrypted envelope version ${String(env.v)} does not match the record binding`,
    );

  return envelopeToAttributeValue(env);
}

function parseEnvelope(av: AttributeValue): EncryptedEnvelope {
  if (!('M' in av) || !av.M) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Encrypted attribute must be a map',
    );
  }

  const m = av.M;
  const v = m.v;
  const edk = m.edk;
  const nonce = m.nonce;
  const ct = m.ct;

  if (!v || !('N' in v) || (v.N !== '1' && v.N !== '2')) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Invalid envelope version',
    );
  }
  if (!edk || !('B' in edk) || !edk.B) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Invalid envelope edk',
    );
  }
  if (!nonce || !('B' in nonce) || !nonce.B) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Invalid envelope nonce',
    );
  }
  if (!ct || !('B' in ct) || !ct.B) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Invalid envelope ct',
    );
  }

  return { v: v.N === '2' ? 2 : 1, edk: edk.B, nonce: nonce.B, ct: ct.B };
}

function toBytes(
  schema: Readonly<AttributeSchema>,
  value: unknown,
): Uint8Array {
  if (schema.type !== 'S' && schema.type !== 'B') {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Encrypted fields must be type S or B: ${schema.attribute}`,
    );
  }

  const av = marshalScalar(schema, value);
  return encodeEncryptedPayload(av);
}

function plaintextAttributeValue(
  schema: Readonly<AttributeSchema>,
  bytes: Uint8Array,
): AttributeValue {
  const av = decodeEncryptedPayload(bytes);

  if (schema.type === 'S' && 'S' in av) return av;
  if (schema.type === 'B' && 'B' in av) return av;

  throw new TheorydbError(
    'ErrInvalidEncryptedEnvelope',
    `Decrypted value type mismatch: ${schema.attribute}`,
  );
}
