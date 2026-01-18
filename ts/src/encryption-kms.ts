import crypto from 'node:crypto';

import {
  DecryptCommand,
  GenerateDataKeyCommand,
  type KMSClient,
} from '@aws-sdk/client-kms';

import { TheorydbError } from './errors.js';
import type {
  EncryptedEnvelope,
  EncryptionContext,
  EncryptionProvider,
} from './encryption.js';

export interface AwsKmsEncryptionProviderOptions {
  keyArn: string;
  randomBytes?: (size: number) => Uint8Array;
}

export class AwsKmsEncryptionProvider implements EncryptionProvider {
  private readonly keyArn: string;
  private readonly randomBytes: (size: number) => Uint8Array;

  constructor(
    private readonly kms: Pick<KMSClient, 'send'>,
    opts: AwsKmsEncryptionProviderOptions,
  ) {
    const keyArn = String(opts.keyArn ?? '').trim();
    if (!keyArn) {
      throw new TheorydbError('ErrInvalidModel', 'KMS keyArn is required');
    }

    this.keyArn = keyArn;
    this.randomBytes = opts.randomBytes ?? crypto.randomBytes;
  }

  async encrypt(
    plaintext: Uint8Array,
    ctx: EncryptionContext,
  ): Promise<EncryptedEnvelope> {
    const dataKeyResp = await this.kms.send(
      new GenerateDataKeyCommand({
        KeyId: this.keyArn,
        KeySpec: 'AES_256',
      }),
    );

    const dek = dataKeyResp.Plaintext;
    const edk = dataKeyResp.CiphertextBlob;

    if (!dek || dek.byteLength !== 32) {
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        'KMS returned invalid plaintext data key',
      );
    }
    if (!edk || edk.byteLength === 0) {
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        'KMS returned empty ciphertext data key',
      );
    }

    const nonce = this.randomBytes(12);
    if (nonce.byteLength !== 12) {
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        'Invalid nonce length',
      );
    }

    const cipher = crypto.createCipheriv('aes-256-gcm', dek, nonce);
    cipher.setAAD(aadForAttribute(ctx.attribute));
    const ciphertext = Buffer.concat([
      cipher.update(plaintext),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();

    return {
      v: 1,
      edk: new Uint8Array(edk),
      nonce,
      ct: Buffer.concat([ciphertext, tag]),
    };
  }

  async decrypt(
    envelope: EncryptedEnvelope,
    ctx: EncryptionContext,
  ): Promise<Uint8Array> {
    const decResp = await this.kms.send(
      new DecryptCommand({
        CiphertextBlob: envelope.edk,
        KeyId: this.keyArn,
      }),
    );

    const dek = decResp.Plaintext;
    if (!dek || dek.byteLength !== 32) {
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        'KMS returned invalid plaintext data key',
      );
    }

    const data = Buffer.from(envelope.ct);
    if (data.length < 16) {
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        'Ciphertext too short',
      );
    }
    const tag = data.subarray(data.length - 16);
    const ciphertext = data.subarray(0, data.length - 16);

    const decipher = crypto.createDecipheriv(
      'aes-256-gcm',
      dek,
      envelope.nonce,
    );
    decipher.setAAD(aadForAttribute(ctx.attribute));
    decipher.setAuthTag(tag);

    return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
  }
}

function aadForAttribute(attributeName: string): Uint8Array {
  return new TextEncoder().encode(
    `theorydb:encrypted:v1|attr=${attributeName}`,
  );
}
