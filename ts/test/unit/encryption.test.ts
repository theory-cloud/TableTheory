import assert from 'node:assert/strict';
import crypto from 'node:crypto';

import type { DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import {
  decryptItemAttributes,
  marshalPutItemEncrypted,
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
  async encrypt(plaintext) {
    const edk = crypto.randomBytes(32);
    const key = crypto.createHmac('sha256', master).update(edk).digest();
    const nonce = crypto.randomBytes(12);
    const cipher = crypto.createCipheriv('aes-256-gcm', key, nonce);
    const ciphertext = Buffer.concat([
      cipher.update(plaintext),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();
    return { v: 1, edk, nonce, ct: Buffer.concat([ciphertext, tag]) };
  },
  async decrypt(envelope) {
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
    decipher.setAuthTag(tag);
    return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
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
