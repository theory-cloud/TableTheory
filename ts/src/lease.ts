import {
  DeleteItemCommand,
  PutItemCommand,
  UpdateItemCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';
import { randomUUID } from 'node:crypto';

import { mapDynamoError } from './dynamo-error.js';
import { TheorydbError } from './errors.js';
import type { SendOptions } from './send-options.js';

export type LeaseKey = {
  pk: string;
  sk: string;
};

export type Lease = {
  key: LeaseKey;
  token: string;
  expiresAt: number;
};

export class LeaseManager {
  private readonly now: () => number;
  private readonly token: () => string;
  private readonly pkAttr: string;
  private readonly skAttr: string;
  private readonly tokenAttr: string;
  private readonly expiresAtAttr: string;
  private readonly ttlAttr: string;
  private readonly ttlBufferSeconds: number;
  private readonly sendOptions: SendOptions | undefined;

  constructor(
    private readonly ddb: DynamoDBClient,
    private readonly tableName: string,
    opts: {
      now?: () => number;
      token?: () => string;
      pkAttr?: string;
      skAttr?: string;
      tokenAttr?: string;
      expiresAtAttr?: string;
      ttlAttr?: string;
      ttlBufferSeconds?: number;
      sendOptions?: SendOptions;
    } = {},
  ) {
    if (!tableName) throw new Error('tableName is required');

    this.now = opts.now ?? (() => Math.floor(Date.now() / 1000));
    this.token = opts.token ?? (() => randomUUID());

    this.pkAttr = opts.pkAttr ?? 'pk';
    this.skAttr = opts.skAttr ?? 'sk';
    this.tokenAttr = opts.tokenAttr ?? 'lease_token';
    this.expiresAtAttr = opts.expiresAtAttr ?? 'lease_expires_at';
    this.ttlAttr = opts.ttlAttr ?? 'ttl';
    this.ttlBufferSeconds = opts.ttlBufferSeconds ?? 60 * 60;
    this.sendOptions = opts.sendOptions;
  }

  lockKey(pk: string, sk = 'LOCK'): LeaseKey {
    return { pk, sk };
  }

  async acquire(key: LeaseKey, opts: { leaseSeconds: number }): Promise<Lease> {
    if (!key?.pk || !key?.sk) throw new Error('key.pk and key.sk are required');
    if (!Number.isFinite(opts.leaseSeconds) || opts.leaseSeconds <= 0) {
      throw new Error('leaseSeconds must be > 0');
    }

    const now = this.now();
    const expiresAt = now + Math.ceil(opts.leaseSeconds);
    const token = this.token();
    const ttl = expiresAt + Math.max(0, Math.ceil(this.ttlBufferSeconds));

    const cmd = new PutItemCommand({
      TableName: this.tableName,
      Item: {
        [this.pkAttr]: { S: key.pk },
        [this.skAttr]: { S: key.sk },
        [this.tokenAttr]: { S: token },
        [this.expiresAtAttr]: { N: String(expiresAt) },
        ...(this.ttlAttr && this.ttlBufferSeconds > 0
          ? { [this.ttlAttr]: { N: String(ttl) } }
          : {}),
      },
      ConditionExpression: 'attribute_not_exists(#pk) OR #exp <= :now',
      ExpressionAttributeNames: {
        '#pk': this.pkAttr,
        '#exp': this.expiresAtAttr,
      },
      ExpressionAttributeValues: {
        ':now': { N: String(now) },
      },
    });

    try {
      await this.ddb.send(cmd, this.sendOptions);
      return { key, token, expiresAt };
    } catch (err) {
      const mapped = mapDynamoError(err);
      if (mapped instanceof TheorydbError && mapped.code === 'ErrConditionFailed') {
        throw new TheorydbError('ErrLeaseHeld', 'Lease held', { cause: err });
      }
      throw mapped;
    }
  }

  async refresh(
    lease: Lease,
    opts: { leaseSeconds: number },
  ): Promise<Lease> {
    if (!lease?.key?.pk || !lease?.key?.sk) {
      throw new Error('lease.key.pk and lease.key.sk are required');
    }
    if (!lease?.token) throw new Error('lease.token is required');
    if (!Number.isFinite(opts.leaseSeconds) || opts.leaseSeconds <= 0) {
      throw new Error('leaseSeconds must be > 0');
    }

    const now = this.now();
    const expiresAt = now + Math.ceil(opts.leaseSeconds);
    const ttl = expiresAt + Math.max(0, Math.ceil(this.ttlBufferSeconds));

    const updateExpression =
      this.ttlAttr && this.ttlBufferSeconds > 0
        ? 'SET #exp = :exp, #ttl = :ttl'
        : 'SET #exp = :exp';

    const cmd = new UpdateItemCommand({
      TableName: this.tableName,
      Key: {
        [this.pkAttr]: { S: lease.key.pk },
        [this.skAttr]: { S: lease.key.sk },
      },
      UpdateExpression: updateExpression,
      ConditionExpression: '#tok = :tok AND #exp > :now',
      ExpressionAttributeNames: {
        '#tok': this.tokenAttr,
        '#exp': this.expiresAtAttr,
        ...(this.ttlAttr && this.ttlBufferSeconds > 0 ? { '#ttl': this.ttlAttr } : {}),
      },
      ExpressionAttributeValues: {
        ':tok': { S: lease.token },
        ':now': { N: String(now) },
        ':exp': { N: String(expiresAt) },
        ...(this.ttlAttr && this.ttlBufferSeconds > 0 ? { ':ttl': { N: String(ttl) } } : {}),
      },
    });

    try {
      await this.ddb.send(cmd, this.sendOptions);
      return { ...lease, expiresAt };
    } catch (err) {
      const mapped = mapDynamoError(err);
      if (mapped instanceof TheorydbError && mapped.code === 'ErrConditionFailed') {
        throw new TheorydbError('ErrLeaseNotOwned', 'Lease not owned', {
          cause: err,
        });
      }
      throw mapped;
    }
  }

  async release(lease: Lease): Promise<void> {
    if (!lease?.key?.pk || !lease?.key?.sk) {
      throw new Error('lease.key.pk and lease.key.sk are required');
    }
    if (!lease?.token) throw new Error('lease.token is required');

    const cmd = new DeleteItemCommand({
      TableName: this.tableName,
      Key: {
        [this.pkAttr]: { S: lease.key.pk },
        [this.skAttr]: { S: lease.key.sk },
      },
      ConditionExpression: '#tok = :tok',
      ExpressionAttributeNames: { '#tok': this.tokenAttr },
      ExpressionAttributeValues: { ':tok': { S: lease.token } },
    });

    try {
      await this.ddb.send(cmd, this.sendOptions);
    } catch (err) {
      const mapped = mapDynamoError(err);
      if (mapped instanceof TheorydbError && mapped.code === 'ErrConditionFailed') {
        return; // best-effort
      }
      throw mapped;
    }
  }
}

