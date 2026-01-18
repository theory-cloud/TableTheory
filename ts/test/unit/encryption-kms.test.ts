import assert from 'node:assert/strict';

import { DecryptCommand, GenerateDataKeyCommand } from '@aws-sdk/client-kms';

import { AwsKmsEncryptionProvider } from '../../src/encryption-kms.js';
import { TheorydbError } from '../../src/errors.js';

const keyArn = 'arn:aws:kms:us-east-1:111111111111:key/test';
const dek = new Uint8Array(32).fill(0x01);
const edk = new Uint8Array([0x02, 0x03, 0x04]);
const nonce = new Uint8Array(12).fill(0x05);

class StubKms {
  calls: unknown[] = [];

  async send(cmd: unknown): Promise<unknown> {
    this.calls.push(cmd);

    if (cmd instanceof GenerateDataKeyCommand) {
      assert.equal(cmd.input.KeyId, keyArn);
      assert.equal(cmd.input.KeySpec, 'AES_256');
      return { Plaintext: dek, CiphertextBlob: edk };
    }

    if (cmd instanceof DecryptCommand) {
      assert.equal(cmd.input.KeyId, keyArn);
      assert.deepEqual(cmd.input.CiphertextBlob, edk);
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
