import test from "node:test";
import assert from "node:assert/strict";
import crypto from "node:crypto";

import {
  decryptItemAttributes,
  marshalPutItemEncrypted,
  type EncryptionContext,
  type EncryptionProvider,
} from "../../../../ts/src/encryption.js";
import { defineModel } from "../../../../ts/src/model.js";
import { unmarshalItem } from "../../../../ts/src/marshal.js";

const master = crypto
  .createHash("sha256")
  .update("theorydb-contract-master")
  .digest();

function canonicalKey(av: { S?: string; N?: string; B?: Uint8Array }): string {
  if (av.S !== undefined) return `S:${av.S}`;
  if (av.N !== undefined) return `N:${av.N}`;
  if (av.B !== undefined) return `B:${Buffer.from(av.B).toString("base64")}`;
  return "";
}

function aad(ctx: EncryptionContext, version: 1 | 2): Buffer {
  if (version === 1) {
    return Buffer.from(`theorydb:encrypted:v1|attr=${ctx.attribute}`, "utf8");
  }
  const binding = ctx.binding;
  assert.ok(binding, "v2 requires a record binding");
  let text =
    `theorydb:encrypted:v2|attr=${ctx.attribute}` +
    `|table=${binding.table}` +
    `|pk=${canonicalKey(binding.partitionKey)}`;
  if (binding.sortKey !== undefined) {
    text += `|sk=${canonicalKey(binding.sortKey)}`;
  }
  return Buffer.from(text, "utf8");
}

const provider: EncryptionProvider = {
  async encrypt(plaintext, ctx) {
    const edk = crypto.randomBytes(32);
    const key = crypto.createHmac("sha256", master).update(edk).digest();
    const nonce = crypto.randomBytes(12);
    const version = ctx.binding === undefined ? 1 : 2;
    const cipher = crypto.createCipheriv("aes-256-gcm", key, nonce);
    cipher.setAAD(aad(ctx, version));
    const ciphertext = Buffer.concat([
      cipher.update(plaintext),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();
    return { v: version, edk, nonce, ct: Buffer.concat([ciphertext, tag]) };
  },
  async decrypt(envelope, ctx) {
    const key = crypto
      .createHmac("sha256", master)
      .update(envelope.edk)
      .digest();
    const data = Buffer.from(envelope.ct);
    const tag = data.subarray(data.length - 16);
    const ciphertext = data.subarray(0, data.length - 16);
    const decipher = crypto.createDecipheriv(
      "aes-256-gcm",
      key,
      envelope.nonce,
    );
    decipher.setAAD(aad(ctx, envelope.v));
    decipher.setAuthTag(tag);
    return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
  },
};

test("encryption envelope shape + AAD binding", async () => {
  const model = defineModel({
    name: "Enc",
    table: { name: "enc_contract" },
    keys: {
      partition: { attribute: "PK", type: "S" },
      sort: { attribute: "SK", type: "S" },
    },
    attributes: [
      { attribute: "PK", type: "S", roles: ["pk"] },
      { attribute: "SK", type: "S", roles: ["sk"] },
      { attribute: "secret", type: "S", encryption: { v: 1 } },
      { attribute: "createdAt", type: "S", roles: ["created_at"] },
      { attribute: "updatedAt", type: "S", roles: ["updated_at"] },
      { attribute: "version", type: "N", roles: ["version"] },
    ],
  });

  const raw = await marshalPutItemEncrypted(
    model,
    { PK: "USER#1", SK: "PROFILE", secret: "top-secret" },
    provider,
    { now: "2026-01-16T00:00:00.000000000Z" },
  );
  assert.ok(raw.secret);
  assert.ok("M" in raw.secret && raw.secret.M);
  assert.equal(raw.secret.M.v?.N, "2");
  assert.ok(raw.secret.M.edk?.B);
  assert.ok(raw.secret.M.nonce?.B);
  assert.ok(raw.secret.M.ct?.B);

  const decrypted = await decryptItemAttributes(model, raw, provider);
  const item = unmarshalItem(model, decrypted);
  assert.equal(item.secret, "top-secret");

  const env = {
    v: 2 as const,
    edk: raw.secret.M.edk!.B!,
    nonce: raw.secret.M.nonce!.B!,
    ct: raw.secret.M.ct!.B!,
  };
  await assert.rejects(() =>
    provider.decrypt(env, {
      model: "Enc",
      attribute: "other",
      binding: {
        table: "enc_contract",
        partitionKey: { S: "USER#1" },
        sortKey: { S: "PROFILE" },
      },
    }),
  );
});
