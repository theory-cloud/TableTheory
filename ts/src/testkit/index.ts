import crypto from 'node:crypto';

import type { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import {
  BatchGetItemCommand,
  BatchWriteItemCommand,
  DeleteItemCommand,
  GetItemCommand,
  PutItemCommand,
  QueryCommand,
  ScanCommand,
  TransactWriteItemsCommand,
  UpdateItemCommand,
  type BatchGetItemCommandOutput,
  type BatchWriteItemCommandOutput,
  type DeleteItemCommandOutput,
  type GetItemCommandOutput,
  type PutItemCommandOutput,
  type QueryCommandOutput,
  type ScanCommandOutput,
  type TransactWriteItemsCommandOutput,
  type UpdateItemCommandOutput,
} from '@aws-sdk/client-dynamodb';

import type { EncryptedEnvelope, EncryptionProvider } from '../encryption.js';

export type SupportedDynamoCommandCtor =
  | typeof PutItemCommand
  | typeof GetItemCommand
  | typeof UpdateItemCommand
  | typeof DeleteItemCommand
  | typeof QueryCommand
  | typeof ScanCommand
  | typeof BatchGetItemCommand
  | typeof BatchWriteItemCommand
  | typeof TransactWriteItemsCommand;

export type SupportedDynamoCommand = InstanceType<SupportedDynamoCommandCtor>;

export type SupportedDynamoOutput<C extends SupportedDynamoCommand> =
  C extends PutItemCommand
    ? PutItemCommandOutput
    : C extends GetItemCommand
      ? GetItemCommandOutput
      : C extends UpdateItemCommand
        ? UpdateItemCommandOutput
        : C extends DeleteItemCommand
          ? DeleteItemCommandOutput
          : C extends QueryCommand
            ? QueryCommandOutput
            : C extends ScanCommand
              ? ScanCommandOutput
              : C extends BatchGetItemCommand
                ? BatchGetItemCommandOutput
                : C extends BatchWriteItemCommand
                  ? BatchWriteItemCommandOutput
                  : C extends TransactWriteItemsCommand
                    ? TransactWriteItemsCommandOutput
                    : never;

export type SupportedDynamoOutputForCtor<C extends SupportedDynamoCommandCtor> =
  C extends typeof PutItemCommand
    ? PutItemCommandOutput
    : C extends typeof GetItemCommand
      ? GetItemCommandOutput
      : C extends typeof UpdateItemCommand
        ? UpdateItemCommandOutput
        : C extends typeof DeleteItemCommand
          ? DeleteItemCommandOutput
          : C extends typeof QueryCommand
            ? QueryCommandOutput
            : C extends typeof ScanCommand
              ? ScanCommandOutput
              : C extends typeof BatchGetItemCommand
                ? BatchGetItemCommandOutput
                : C extends typeof BatchWriteItemCommand
                  ? BatchWriteItemCommandOutput
                  : C extends typeof TransactWriteItemsCommand
                    ? TransactWriteItemsCommandOutput
                    : never;

export interface MockDynamoDBClient {
  readonly client: DynamoDBClient;
  readonly calls: SupportedDynamoCommand[];

  when<C extends SupportedDynamoCommandCtor>(
    ctor: C,
    handler: (
      cmd: InstanceType<C>,
    ) =>
      | Promise<SupportedDynamoOutputForCtor<C>>
      | SupportedDynamoOutputForCtor<C>,
  ): void;

  reset(): void;
}

export function createMockDynamoDBClient(): MockDynamoDBClient {
  const calls: SupportedDynamoCommand[] = [];
  const handlersByCtor = new Map<
    SupportedDynamoCommandCtor,
    (cmd: SupportedDynamoCommand) => Promise<unknown>
  >();
  const handlersByName = new Map<
    string,
    (cmd: SupportedDynamoCommand) => Promise<unknown>
  >();

  const client = {
    send: async (command: SupportedDynamoCommand): Promise<unknown> => {
      calls.push(command);

      const ctor = command.constructor as SupportedDynamoCommandCtor;
      const name = (ctor as { name?: unknown }).name;
      const handler =
        handlersByCtor.get(ctor) ??
        (typeof name === 'string' ? handlersByName.get(name) : undefined);
      if (!handler) {
        throw new Error(
          `No handler registered for DynamoDB command: ${String(name ?? ctor)}`,
        );
      }
      return await handler(command);
    },
  } as unknown as DynamoDBClient;

  return {
    client,
    calls,
    when<C extends SupportedDynamoCommandCtor>(
      ctor: C,
      handler: (
        cmd: InstanceType<C>,
      ) =>
        | Promise<SupportedDynamoOutputForCtor<C>>
        | SupportedDynamoOutputForCtor<C>,
    ): void {
      const wrapped = async (cmd: SupportedDynamoCommand): Promise<unknown> =>
        await handler(cmd as InstanceType<C>);
      handlersByCtor.set(ctor, wrapped);

      const name = (ctor as { name?: unknown }).name;
      if (typeof name === 'string' && name.length > 0) {
        handlersByName.set(name, wrapped);
      }
    },
    reset(): void {
      calls.length = 0;
      handlersByCtor.clear();
      handlersByName.clear();
    },
  };
}

export function fixedNow(now: string): () => string {
  return () => now;
}

export function createDeterministicEncryptionProvider(
  seed: string,
): EncryptionProvider {
  const master = crypto.createHash('sha256').update(seed, 'utf8').digest();
  let counter = 0;

  const nextBytes = (label: string, length: number): Buffer => {
    counter += 1;
    const digest = crypto
      .createHmac('sha256', master)
      .update(`${label}|${counter}`, 'utf8')
      .digest();
    return digest.subarray(0, length);
  };

  const aad = (attr: string): Buffer =>
    Buffer.from(`theorydb:encrypted:v1|attr=${attr}`, 'utf8');

  const keyFromEDK = (edk: Uint8Array): Buffer =>
    crypto.createHmac('sha256', master).update(edk).digest();

  const parseCiphertext = (
    ct: Uint8Array,
  ): { ciphertext: Buffer; tag: Buffer } => {
    const data = Buffer.from(ct);
    if (data.length < 17) throw new Error('invalid ciphertext');
    const tag = data.subarray(data.length - 16);
    const ciphertext = data.subarray(0, data.length - 16);
    return { ciphertext, tag };
  };

  return {
    async encrypt(plaintext, ctx): Promise<EncryptedEnvelope> {
      const edk = nextBytes(`edk|${ctx.model}|${ctx.attribute}`, 32);
      const key = keyFromEDK(edk);
      const nonce = nextBytes(`nonce|${ctx.model}|${ctx.attribute}`, 12);

      const cipher = crypto.createCipheriv('aes-256-gcm', key, nonce);
      cipher.setAAD(aad(ctx.attribute));
      const ciphertext = Buffer.concat([
        cipher.update(plaintext),
        cipher.final(),
      ]);
      const tag = cipher.getAuthTag();

      return { v: 1, edk, nonce, ct: Buffer.concat([ciphertext, tag]) };
    },

    async decrypt(envelope, ctx): Promise<Uint8Array> {
      const key = keyFromEDK(envelope.edk);
      const { ciphertext, tag } = parseCiphertext(envelope.ct);

      const decipher = crypto.createDecipheriv(
        'aes-256-gcm',
        key,
        envelope.nonce,
      );
      decipher.setAAD(aad(ctx.attribute));
      decipher.setAuthTag(tag);
      return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
    },
  };
}
