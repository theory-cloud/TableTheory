import assert from 'node:assert/strict';
import test from 'node:test';

import { DecryptCommand, GenerateDataKeyCommand } from '@aws-sdk/client-kms';

import { AwsKmsEncryptionProvider } from '../../src/encryption-kms.js';
import { aadForContext } from '../../src/encryption.js';
import { TheorydbError } from '../../src/errors.js';

const keyArn = 'arn:aws:kms:us-east-1:111111111111:key/test';
const dek = new Uint8Array(32).fill(0x01);
const edk = new Uint8Array([0x02, 0x03, 0x04]);
const nonce = new Uint8Array(12).fill(0x05);

class StubKms {
  calls: unknown[] = [];
  generateContexts: (Record<string, string> | undefined)[] = [];
  decryptContexts: (Record<string, string> | undefined)[] = [];

  async send(cmd: unknown): Promise<unknown> {
    this.calls.push(cmd);

    if (cmd instanceof GenerateDataKeyCommand) {
      assert.equal(cmd.input.KeyId, keyArn);
      assert.equal(cmd.input.KeySpec, 'AES_256');
      this.generateContexts.push(cmd.input.EncryptionContext);
      return { Plaintext: dek, CiphertextBlob: edk };
    }

    if (cmd instanceof DecryptCommand) {
      assert.equal(cmd.input.KeyId, keyArn);
      assert.deepEqual(cmd.input.CiphertextBlob, edk);
      this.decryptContexts.push(cmd.input.EncryptionContext);
      return { Plaintext: dek };
    }

    throw new Error(
      `unexpected command: ${(cmd as { constructor?: { name?: string } })?.constructor?.name}`,
    );
  }
}

{
  assert.throws(
    () => new AwsKmsEncryptionProvider(new StubKms() as never, { keyArn: '' }),
    (err) => {
      return err instanceof TheorydbError && err.code === 'ErrInvalidModel';
    },
  );
}

{
  const kms = new StubKms();
  const provider = new AwsKmsEncryptionProvider(kms as never, {
    keyArn,
    randomBytes: () => nonce,
  });

  const plaintext = new TextEncoder().encode('payload');

  const env = await provider.encrypt(plaintext, {
    model: 'T',
    attribute: 'secret',
  });
  assert.equal(env.v, 1);
  assert.deepEqual(env.edk, edk);
  assert.deepEqual(env.nonce, nonce);
  assert.ok(env.ct.length > 16);
  assert.equal(
    kms.generateContexts.at(-1),
    undefined,
    'v1 unwraps legacy data keys without context',
  );

  const roundTrip = await provider.decrypt(env, {
    model: 'T',
    attribute: 'secret',
  });
  assert.deepEqual(Buffer.from(roundTrip), Buffer.from(plaintext));

  await assert.rejects(
    () =>
      provider.decrypt(env, {
        model: 'T',
        attribute: 'other',
      }),
    () => true,
  );

  assert.equal(kms.calls.length, 3);
}

const isInvalidEnvelope = (err: unknown): boolean =>
  err instanceof TheorydbError && err.code === 'ErrInvalidEncryptedEnvelope';

void test('KMS writes v2 envelopes bound to the record identity', async () => {
  const kms = new StubKms();
  const provider = new AwsKmsEncryptionProvider(kms as never, {
    keyArn,
    randomBytes: () => nonce,
  });
  const plaintext = new TextEncoder().encode('payload');
  const binding = {
    table: 'users_contract',
    partitionKey: { S: 'USER#1' },
    sortKey: { S: 'PROFILE' },
  };

  const env = await provider.encrypt(plaintext, {
    model: 'T',
    attribute: 'secret',
    binding,
  });
  assert.equal(env.v, 2);
  const generateContext = kms.generateContexts.at(-1);
  assert.ok(generateContext);
  assert.equal(
    generateContext['theorydb:encryption:aad'],
    new TextDecoder().decode(
      aadForContext({ model: 'T', attribute: 'secret', binding }, 2),
    ),
  );

  const roundTrip = await provider.decrypt(env, {
    model: 'T',
    attribute: 'secret',
    binding,
  });
  assert.deepEqual(Buffer.from(roundTrip), Buffer.from(plaintext));
  assert.deepEqual(kms.decryptContexts.at(-1), generateContext);

  await assert.rejects(
    () =>
      provider.decrypt(env, {
        model: 'T',
        attribute: 'secret',
        binding: { ...binding, partitionKey: { S: 'USER#2' } },
      }),
    isInvalidEnvelope,
  );

  await assert.rejects(
    () =>
      provider.decrypt(env, {
        model: 'T',
        attribute: 'secret',
        binding: { ...binding, table: 'other_table' },
      }),
    isInvalidEnvelope,
  );

  await assert.rejects(
    () =>
      provider.decrypt(env, {
        model: 'T',
        attribute: 'secret',
      }),
    isInvalidEnvelope,
  );

  // These two identities produced byte-identical AAD under raw delimiter
  // concatenation; the length-prefixed v2 format must keep the envelope bound
  // to the first identity only.
  const collidingFirst = {
    table: 'T',
    partitionKey: { S: 'x|sk=S:y' },
    sortKey: { S: 'z' },
  };
  const collidingSecond = {
    table: 'T',
    partitionKey: { S: 'x' },
    sortKey: { S: 'y|sk=S:z' },
  };
  const collidingEnvelope = await provider.encrypt(plaintext, {
    model: 'T',
    attribute: 'secret',
    binding: collidingFirst,
  });
  await assert.rejects(
    () =>
      provider.decrypt(collidingEnvelope, {
        model: 'T',
        attribute: 'secret',
        binding: collidingSecond,
      }),
    isInvalidEnvelope,
  );
});

void test('KMS v2 without a binding fails before calling KMS', async () => {
  const kms = new StubKms();
  const provider = new AwsKmsEncryptionProvider(kms as never, {
    keyArn,
    randomBytes: () => nonce,
  });
  const binding = { table: 'users_contract', partitionKey: { S: 'USER#1' } };
  const env = await provider.encrypt(new TextEncoder().encode('payload'), {
    model: 'T',
    attribute: 'secret',
    binding,
  });
  const callsAfterEncrypt = kms.calls.length;

  await assert.rejects(
    () => provider.decrypt(env, { model: 'T', attribute: 'secret' }),
    isInvalidEnvelope,
  );
  assert.equal(kms.calls.length, callsAfterEncrypt);
});

void test('KMS still decrypts a legacy v1 envelope when a binding is supplied', async () => {
  const kms = new StubKms();
  const provider = new AwsKmsEncryptionProvider(kms as never, {
    keyArn,
    randomBytes: () => nonce,
  });
  const plaintext = new TextEncoder().encode('payload');

  const legacy = await provider.encrypt(plaintext, {
    model: 'T',
    attribute: 'secret',
  });
  assert.equal(legacy.v, 1);

  const roundTrip = await provider.decrypt(legacy, {
    model: 'T',
    attribute: 'secret',
    binding: { table: 'users_contract', partitionKey: { S: 'USER#1' } },
  });
  assert.deepEqual(Buffer.from(roundTrip), Buffer.from(plaintext));
});
