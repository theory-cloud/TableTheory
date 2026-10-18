import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import test from 'node:test';

import {
  UpdateItemCommand,
  type AttributeValue,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import {
  aadForContext,
  decryptItemAttributes,
  encryptAttributeValue,
  envelopeVersionForContext,
  marshalPutItemEncrypted,
  type EncryptionContext,
  type EncryptionProvider,
} from '../../src/encryption.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import { unmarshalItem } from '../../src/marshal.js';

const master = crypto
  .createHash('sha256')
  .update('theorydb-test-master')
  .digest();

const provider: EncryptionProvider = {
  async encrypt(plaintext, ctx) {
    const version = envelopeVersionForContext(ctx);
    const edk = crypto.randomBytes(32);
    const key = crypto.createHmac('sha256', master).update(edk).digest();
    const nonce = crypto.randomBytes(12);
    const cipher = crypto.createCipheriv('aes-256-gcm', key, nonce);
    cipher.setAAD(aadForContext(ctx, version));
    const ciphertext = Buffer.concat([
      cipher.update(plaintext),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();
    return { v: version, edk, nonce, ct: Buffer.concat([ciphertext, tag]) };
  },
  async decrypt(envelope, ctx) {
    const key = crypto
      .createHmac('sha256', master)
      .update(envelope.edk)
      .digest();
    const data = Buffer.from(envelope.ct);
    const tag = data.subarray(data.length - 16);
    const ciphertext = data.subarray(0, data.length - 16);
    const decipher = crypto.createDecipheriv(
      'aes-256-gcm',
      key,
      envelope.nonce,
    );
    decipher.setAAD(aadForContext(ctx, envelope.v));
    decipher.setAuthTag(tag);
    try {
      return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
    } catch {
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        'aes-gcm decrypt failed',
      );
    }
  },
};

const encModel = defineModel({
  name: 'Enc',
  table: { name: 'enc_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

{
  const raw = await marshalPutItemEncrypted(
    encModel,
    { PK: 'USER#1', SK: 'PROFILE', secret: 'top-secret' },
    provider,
    { now: '2026-01-16T00:00:00.000000000Z' },
  );
  assert.ok(raw.secret);
  assert.ok('M' in raw.secret);
  assert.ok(raw.secret.M?.ct);
  assert.equal(raw.secret.M?.v?.N, '2');

  const decrypted = await decryptItemAttributes(encModel, raw, provider);
  assert.deepEqual(decrypted.secret, { S: 'top-secret' });

  const item = unmarshalItem(encModel, decrypted);
  assert.equal(item.secret, 'top-secret');
}

{
  class FakeDdb {
    async send(): Promise<never> {
      throw new Error('send should not be called');
    }
  }

  const client = new TheorydbClient(
    new FakeDdb() as unknown as DynamoDBClient,
  ).register(encModel);
  await assert.rejects(
    () => client.create('Enc', { PK: 'USER#1', SK: 'PROFILE', secret: 'x' }),
    (err) =>
      err instanceof TheorydbError && err.code === 'ErrEncryptionNotConfigured',
  );
}

{
  assert.throws(() =>
    defineModel({
      name: 'BadKey',
      table: { name: 'bad' },
      keys: { partition: { attribute: 'PK', type: 'S' } },
      attributes: [
        { attribute: 'PK', type: 'S', roles: ['pk'], encryption: { v: 1 } },
      ],
    }),
  );
}

const decodesAad = (ctx: EncryptionContext, version: 1 | 2): string =>
  new TextDecoder().decode(aadForContext(ctx, version));

const isInvalidEnvelope = (err: unknown): boolean =>
  err instanceof TheorydbError && err.code === 'ErrInvalidEncryptedEnvelope';

void test('AAD strings match the cross-runtime contract', () => {
  assert.equal(
    decodesAad({ model: 'Enc', attribute: 'secret' }, 1),
    'theorydb:encrypted:v1|attr=secret',
  );
  assert.equal(
    decodesAad(
      {
        model: 'Enc',
        attribute: 'secret',
        binding: {
          table: 'enc_contract',
          partitionKey: { S: 'USER#1' },
          sortKey: { S: 'PROFILE' },
        },
      },
      2,
    ),
    'theorydb:encrypted:v2|attr=6:secret|table=12:enc_contract|pk=8:S:USER#1|sk=9:S:PROFILE',
  );
  assert.equal(
    decodesAad(
      {
        model: 'Enc',
        attribute: 'secret',
        binding: { table: 'enc_contract', partitionKey: { N: '42' } },
      },
      2,
    ),
    'theorydb:encrypted:v2|attr=6:secret|table=12:enc_contract|pk=4:N:42',
  );
  assert.equal(
    decodesAad(
      {
        model: 'Enc',
        attribute: 'secret',
        binding: {
          table: 'enc_contract',
          partitionKey: { B: new Uint8Array([1, 2, 3]) },
          sortKey: { B: new Uint8Array([4, 5, 6]) },
        },
      },
      2,
    ),
    'theorydb:encrypted:v2|attr=6:secret|table=12:enc_contract|pk=6:B:AQID|sk=6:B:BAUG',
  );
});

void test('v2 AAD length prefixes prevent delimiter collisions', () => {
  const first = decodesAad(
    {
      model: 'Enc',
      attribute: 'secret',
      binding: {
        table: 'T',
        partitionKey: { S: 'x|sk=S:y' },
        sortKey: { S: 'z' },
      },
    },
    2,
  );
  const second = decodesAad(
    {
      model: 'Enc',
      attribute: 'secret',
      binding: {
        table: 'T',
        partitionKey: { S: 'x' },
        sortKey: { S: 'y|sk=S:z' },
      },
    },
    2,
  );

  assert.notEqual(first, second);
  assert.equal(
    first,
    'theorydb:encrypted:v2|attr=6:secret|table=1:T|pk=10:S:x|sk=S:y|sk=3:S:z',
  );
  assert.equal(
    second,
    'theorydb:encrypted:v2|attr=6:secret|table=1:T|pk=3:S:x|sk=10:S:y|sk=S:z',
  );
});

void test('unsupported key types fail closed instead of canonicalizing to empty', () => {
  assert.throws(
    () =>
      decodesAad(
        {
          model: 'Enc',
          attribute: 'secret',
          binding: { table: 'T', partitionKey: { NULL: true } },
        },
        2,
      ),
    isInvalidEnvelope,
  );
  assert.throws(
    () =>
      decodesAad(
        {
          model: 'Enc',
          attribute: 'secret',
          binding: { table: 'T', partitionKey: { BOOL: true } },
        },
        2,
      ),
    isInvalidEnvelope,
  );
});

void test('v2 binds to one record: another record key cannot decrypt it', async () => {
  const raw = await marshalPutItemEncrypted(
    encModel,
    { PK: 'USER#1', SK: 'PROFILE', secret: 'top-secret' },
    provider,
    { now: '2026-01-16T00:00:00.000000000Z' },
  );

  const replayed: Record<string, AttributeValue> = {
    PK: { S: 'USER#2' },
    SK: { S: 'PROFILE' },
  };
  const replayedSecret = raw.secret;
  assert.ok(replayedSecret);
  replayed.secret = replayedSecret;

  await assert.rejects(
    () => decryptItemAttributes(encModel, replayed, provider),
    isInvalidEnvelope,
  );

  const replayedSortKey: Record<string, AttributeValue> = {
    PK: { S: 'USER#1' },
    SK: { S: 'OTHER' },
    secret: replayedSecret,
  };
  await assert.rejects(
    () => decryptItemAttributes(encModel, replayedSortKey, provider),
    isInvalidEnvelope,
  );
});

void test('v2 binds to one table: another table cannot decrypt it', async () => {
  const otherTableModel = defineModel({
    name: 'EncOther',
    table: { name: 'other_table' },
    keys: {
      partition: { attribute: 'PK', type: 'S' },
      sort: { attribute: 'SK', type: 'S' },
    },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'SK', type: 'S', roles: ['sk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });

  const raw = await marshalPutItemEncrypted(
    encModel,
    { PK: 'USER#1', SK: 'PROFILE', secret: 'top-secret' },
    provider,
  );
  const env = raw.secret;
  assert.ok(env);

  await assert.rejects(
    () =>
      decryptItemAttributes(
        otherTableModel,
        { PK: { S: 'USER#1' }, SK: { S: 'PROFILE' }, secret: env },
        provider,
      ),
    isInvalidEnvelope,
  );
});

void test('legacy v1 envelopes still decrypt without and with a binding', async () => {
  const secretSchema = encModel.attributes.get('secret');
  assert.ok(secretSchema);

  const legacy = await encryptAttributeValue(
    secretSchema,
    'legacy-secret',
    provider,
    { model: encModel.name, attribute: 'secret' },
  );
  assert.ok('M' in legacy);
  assert.equal(legacy.M?.v?.N, '1');

  const decrypted = await decryptItemAttributes(
    encModel,
    { PK: { S: 'USER#1' }, SK: { S: 'PROFILE' }, secret: legacy },
    provider,
  );
  assert.deepEqual(decrypted.secret, { S: 'legacy-secret' });

  // A projection that omits the keys still reads v1 data.
  const projected = await decryptItemAttributes(
    encModel,
    { secret: legacy },
    provider,
  );
  assert.deepEqual(projected.secret, { S: 'legacy-secret' });
});

void test('a v2 envelope fails closed without its record binding', async () => {
  const binding = {
    table: 'enc_contract',
    partitionKey: { S: 'USER#1' },
    sortKey: { S: 'PROFILE' },
  };
  const env = await provider.encrypt(new TextEncoder().encode('payload'), {
    model: encModel.name,
    attribute: 'secret',
    binding,
  });
  assert.equal(env.v, 2);

  await assert.rejects(
    () =>
      provider.decrypt(env, {
        model: encModel.name,
        attribute: 'secret',
      }),
    isInvalidEnvelope,
  );

  const raw = await marshalPutItemEncrypted(
    encModel,
    { PK: 'USER#1', SK: 'PROFILE', secret: 'top-secret' },
    provider,
  );
  const bound = raw.secret;
  assert.ok(bound);
  await assert.rejects(
    () => decryptItemAttributes(encModel, { secret: bound }, provider),
    isInvalidEnvelope,
  );

  assert.throws(
    () => aadForContext({ model: encModel.name, attribute: 'secret' }, 2),
    isInvalidEnvelope,
  );
});

void test('a v2 envelope still rejects a different attribute name', async () => {
  const binding = {
    table: 'enc_contract',
    partitionKey: { S: 'USER#1' },
    sortKey: { S: 'PROFILE' },
  };
  const env = await provider.encrypt(new TextEncoder().encode('payload'), {
    model: encModel.name,
    attribute: 'secret',
    binding,
  });

  await assert.rejects(
    () =>
      provider.decrypt(env, {
        model: encModel.name,
        attribute: 'other',
        binding,
      }),
    isInvalidEnvelope,
  );
});

void test('client.update writes v2 envelopes bound to the record key', async () => {
  let sent: UpdateItemCommand | undefined;
  class CapturingDdb {
    async send(cmd: unknown): Promise<unknown> {
      sent = cmd as UpdateItemCommand;
      return {};
    }
  }

  const client = new TheorydbClient(
    new CapturingDdb() as unknown as DynamoDBClient,
    { encryption: provider },
  ).register(encModel);

  await client.update(
    'Enc',
    { PK: 'USER#1', SK: 'PROFILE', version: '0', secret: 'rotated' },
    ['secret'],
  );

  assert.ok(sent);
  const envelope = sent.input.ExpressionAttributeValues?.[':v1'];
  assert.ok(envelope && 'M' in envelope);
  assert.equal(envelope.M?.v?.N, '2');

  const decrypted = await decryptItemAttributes(
    encModel,
    {
      PK: { S: 'USER#1' },
      SK: { S: 'PROFILE' },
      secret: envelope,
    },
    provider,
  );
  assert.deepEqual(decrypted.secret, { S: 'rotated' });
});
