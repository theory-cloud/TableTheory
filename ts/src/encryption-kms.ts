import crypto from 'node:crypto';

import {
  DecryptCommand,
  GenerateDataKeyCommand,
  type KMSClient,
} from '@aws-sdk/client-kms';

import {
  aadForContext,
  envelopeVersionForContext,
  type EncryptedEnvelope,
  type EncryptionContext,
  type EncryptionProvider,
} from './encryption.js';
import { TheorydbError } from './errors.js';

const kmsEncryptionContextAADKey = 'theorydb:encryption:aad';

/**
 * The KMS EncryptionContext for an envelope version. v2 envelopes bind the
 * same AAD into the wrapped data key, so KMS itself refuses to unwrap a data
 * key for a different record. v1 envelopes carry no context so existing data
 * stays readable.
 */
function kmsEncryptionContext(
  aad: Uint8Array,
  version: 1 | 2,
): Record<string, string> | undefined {
  if (version !== 2) return undefined;
  return { [kmsEncryptionContextAADKey]: new TextDecoder().decode(aad) };
}

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
    const version = envelopeVersionForContext(ctx);
    const aad = aadForContext(ctx, version);

    const dataKeyResp = await this.kms.send(
      new GenerateDataKeyCommand({
        KeyId: this.keyArn,
        KeySpec: 'AES_256',
        EncryptionContext: kmsEncryptionContext(aad, version),
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
    cipher.setAAD(aad);
    const ciphertext = Buffer.concat([
      cipher.update(plaintext),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();

    return {
      v: version,
      edk: new Uint8Array(edk),
      nonce,
      ct: Buffer.concat([ciphertext, tag]),
    };
  }

  async decrypt(
    envelope: EncryptedEnvelope,
    ctx: EncryptionContext,
  ): Promise<Uint8Array> {
    // A v2 envelope that lost its record binding must fail before any KMS or
    // ciphertext work happens.
    const aad = aadForContext(ctx, envelope.v);

    const decResp = await this.kms.send(
      new DecryptCommand({
        CiphertextBlob: envelope.edk,
        KeyId: this.keyArn,
        EncryptionContext: kmsEncryptionContext(aad, envelope.v),
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
    decipher.setAAD(aad);
    decipher.setAuthTag(tag);

    try {
      return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
    } catch {
      throw new TheorydbError(
        'ErrInvalidEncryptedEnvelope',
        'aes-gcm decrypt failed',
      );
    }
  }
}
