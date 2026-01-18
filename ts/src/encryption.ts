import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';
import type { AttributeSchema, Model } from './model.js';
import {
  isEmpty,
  marshalKey,
  marshalScalar,
  nowRfc3339Nano,
} from './marshal.js';
import {
  decodeEncryptedPayload,
  encodeEncryptedPayload,
} from './encryption-avjson.js';

export interface EncryptionContext {
  model: string;
  attribute: string;
}

export interface EncryptedEnvelope {
  v: 1;
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
  Object.assign(out, marshalKey(model, item));

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
    if (attr.omit_empty && isEmpty(value)) continue;

    if (attr.encryption !== undefined) {
      out[name] = await encryptAttributeValue(attr, value, provider, {
        model: model.name,
        attribute: name,
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
): Promise<Record<string, AttributeValue>> {
  const out: Record<string, AttributeValue> = { ...item };

  for (const attr of model.schema.attributes) {
    if (attr.encryption === undefined) continue;

    const name = attr.attribute;
    const av = out[name];
    if (!av) continue;

    const env = parseEnvelope(av);
    const plaintext = await provider.decrypt(env, {
      model: model.name,
      attribute: name,
    });
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
  const env = await provider.encrypt(bytes, ctx);
  if (env.v !== 1)
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Unsupported envelope version',
    );

  return {
    M: {
      v: { N: '1' },
      edk: { B: env.edk },
      nonce: { B: env.nonce },
      ct: { B: env.ct },
    },
  };
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

  if (!v || !('N' in v) || v.N !== '1') {
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

  return { v: 1, edk: edk.B, nonce: nonce.B, ct: ct.B };
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
