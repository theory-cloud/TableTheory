import type { DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from './client.js';
import { TheorydbError } from './errors.js';
import { LeaseManager } from './lease.js';
import { defineModel, type Model } from './model.js';

export type FaceTheoryIsrMeta = {
  htmlPointer: string;
  generatedAtMs: number;
  revalidateSeconds: number;
  etag?: string;
};

export type FaceTheoryIsrLease = {
  leaseToken: string;
  leaseExpiresAtMs: number;
};

export type FaceTheoryIsrMetaStoreGetArgs = {
  cacheKey: string;
};

export type FaceTheoryIsrMetaStoreTryAcquireLeaseArgs = {
  cacheKey: string;
  leaseOwner?: string;
  nowMs?: number;
  leaseDurationMs: number;
};

export type FaceTheoryIsrMetaStoreCommitGenerationArgs = {
  cacheKey: string;
  leaseOwner?: string;
  leaseToken: string;
  nowMs?: number;
  htmlPointer: string;
  generatedAtMs: number;
  revalidateSeconds: number;
  etag?: string;
  ttlUnixSeconds?: number;
};

export type FaceTheoryIsrMetaStoreReleaseLeaseArgs = {
  cacheKey: string;
  leaseOwner?: string;
  leaseToken: string;
};

export type FaceTheoryIsrMetaStoreConfig = {
  ddb: DynamoDBClient;
  tableName: string;

  client?: TheorydbClient;

  pkFromCacheKey?: (cacheKey: string) => string;
  pkPrefix?: string;

  leaseTtlBufferSeconds?: number;
  leaseToken?: () => string;

  metaTtlSeconds?: number;
};

export function defineFaceTheoryCacheMetadataModel(tableName: string): Model {
  return defineModel({
    name: 'FaceTheoryCacheMetadata',
    table: { name: tableName },
    keys: {
      partition: { attribute: 'pk', type: 'S' },
      sort: { attribute: 'sk', type: 'S' },
    },
    attributes: [
      { attribute: 'pk', type: 'S', roles: ['pk'] },
      { attribute: 'sk', type: 'S', roles: ['sk'] },
      { attribute: 's3_key', type: 'S', required: true },
      { attribute: 'generated_at', type: 'N', required: true },
      { attribute: 'revalidate_seconds', type: 'N', required: true },
      { attribute: 'etag', type: 'S', optional: true },
      { attribute: 'ttl', type: 'N', roles: ['ttl'], optional: true },
    ],
  });
}

export function defineFaceTheoryCacheLeaseModel(tableName: string): Model {
  return defineModel({
    name: 'FaceTheoryCacheLease',
    table: { name: tableName },
    keys: {
      partition: { attribute: 'pk', type: 'S' },
      sort: { attribute: 'sk', type: 'S' },
    },
    attributes: [
      { attribute: 'pk', type: 'S', roles: ['pk'] },
      { attribute: 'sk', type: 'S', roles: ['sk'] },
      { attribute: 'lease_token', type: 'S', required: true },
      { attribute: 'lease_expires_at', type: 'N', required: true },
      { attribute: 'ttl', type: 'N', roles: ['ttl'], optional: true },
    ],
  });
}

const META_SK = 'META';
const LOCK_SK = 'LOCK';

export class FaceTheoryIsrMetaStore {
  private readonly ddb: DynamoDBClient;
  private readonly tableName: string;
  private readonly client: TheorydbClient;
  private readonly pkFromCacheKey: (cacheKey: string) => string;
  private readonly leaseTtlBufferSeconds: number;
  private readonly leaseToken: (() => string) | undefined;
  private readonly metaTtlSeconds: number | undefined;

  constructor(cfg: FaceTheoryIsrMetaStoreConfig) {
    if (!cfg?.ddb) throw new Error('ddb is required');
    if (!cfg?.tableName) throw new Error('tableName is required');

    this.ddb = cfg.ddb;
    this.tableName = cfg.tableName;
    this.leaseTtlBufferSeconds = cfg.leaseTtlBufferSeconds ?? 60 * 60;
    this.leaseToken = cfg.leaseToken;
    this.metaTtlSeconds = cfg.metaTtlSeconds;

    this.pkFromCacheKey =
      cfg.pkFromCacheKey ??
      ((cacheKey) => `${cfg.pkPrefix ?? 'CACHE#'}${cacheKey}`);

    this.client =
      cfg.client ??
      new TheorydbClient(this.ddb).register(
        defineFaceTheoryCacheMetadataModel(this.tableName),
        defineFaceTheoryCacheLeaseModel(this.tableName),
      );

    // Ensure models exist when caller provided a pre-configured client.
    if (cfg.client) {
      this.client.register(
        defineFaceTheoryCacheMetadataModel(this.tableName),
        defineFaceTheoryCacheLeaseModel(this.tableName),
      );
    }
  }

  async get(
    args: FaceTheoryIsrMetaStoreGetArgs,
  ): Promise<FaceTheoryIsrMeta | null> {
    const cacheKey = args.cacheKey;
    const pk = this.pkFromCacheKey(cacheKey);

    try {
      const item = await this.client.get('FaceTheoryCacheMetadata', {
        pk,
        sk: META_SK,
      });
      return {
        htmlPointer: item.s3_key as string,
        generatedAtMs: Math.floor((item.generated_at as number) * 1000),
        revalidateSeconds: item.revalidate_seconds as number,
        ...(typeof item.etag === 'string' ? { etag: item.etag } : {}),
      };
    } catch (err) {
      if (err instanceof TheorydbError && err.code === 'ErrItemNotFound') {
        return null;
      }
      throw err;
    }
  }

  async tryAcquireLease(
    args: FaceTheoryIsrMetaStoreTryAcquireLeaseArgs,
  ): Promise<FaceTheoryIsrLease | null> {
    const cacheKey = args.cacheKey;
    const pk = this.pkFromCacheKey(cacheKey);

    if (!Number.isFinite(args.leaseDurationMs) || args.leaseDurationMs <= 0) {
      throw new Error('leaseDurationMs must be > 0');
    }

    const nowUnix =
      args.nowMs === undefined ? undefined : Math.floor(args.nowMs / 1000);

    const mgr = new LeaseManager(this.ddb, this.tableName, {
      ...(nowUnix === undefined ? {} : { now: () => nowUnix }),
      ...(this.leaseToken ? { token: this.leaseToken } : {}),
      ttlBufferSeconds: this.leaseTtlBufferSeconds,
    });

    try {
      const lease = await mgr.acquire(
        { pk, sk: LOCK_SK },
        {
          leaseSeconds: Math.ceil(args.leaseDurationMs / 1000),
        },
      );
      return {
        leaseToken: lease.token,
        leaseExpiresAtMs: lease.expiresAt * 1000,
      };
    } catch (err) {
      if (err instanceof TheorydbError && err.code === 'ErrLeaseHeld') {
        return null;
      }
      throw err;
    }
  }

  async commitGeneration(
    args: FaceTheoryIsrMetaStoreCommitGenerationArgs,
  ): Promise<void> {
    const cacheKey = args.cacheKey;
    const pk = this.pkFromCacheKey(cacheKey);

    if (!args.leaseToken) throw new Error('leaseToken is required');
    if (!args.htmlPointer) throw new Error('htmlPointer is required');
    if (!Number.isFinite(args.generatedAtMs) || args.generatedAtMs <= 0) {
      throw new Error('generatedAtMs must be > 0');
    }
    if (
      !Number.isFinite(args.revalidateSeconds) ||
      args.revalidateSeconds <= 0
    ) {
      throw new Error('revalidateSeconds must be > 0');
    }

    const nowUnix =
      args.nowMs === undefined
        ? Math.floor(Date.now() / 1000)
        : Math.floor(args.nowMs / 1000);
    const generatedAtUnix = Math.floor(args.generatedAtMs / 1000);

    const ttlUnixSeconds =
      args.ttlUnixSeconds ??
      (this.metaTtlSeconds === undefined
        ? undefined
        : generatedAtUnix + this.metaTtlSeconds);

    try {
      await this.client.transactWrite([
        {
          kind: 'put',
          model: 'FaceTheoryCacheMetadata',
          item: {
            pk,
            sk: META_SK,
            s3_key: args.htmlPointer,
            generated_at: generatedAtUnix,
            revalidate_seconds: args.revalidateSeconds,
            ...(typeof args.etag === 'string' ? { etag: args.etag } : {}),
            ...(ttlUnixSeconds === undefined ? {} : { ttl: ttlUnixSeconds }),
          },
        },
        {
          kind: 'delete',
          model: 'FaceTheoryCacheLease',
          key: { pk, sk: LOCK_SK },
          conditionExpression: '#tok = :tok AND #exp > :now',
          expressionAttributeNames: {
            '#tok': 'lease_token',
            '#exp': 'lease_expires_at',
          },
          expressionAttributeValues: {
            ':tok': { S: args.leaseToken },
            ':now': { N: String(nowUnix) },
          },
        },
      ]);
    } catch (err) {
      if (err instanceof TheorydbError && err.code === 'ErrConditionFailed') {
        throw new TheorydbError('ErrLeaseNotOwned', 'Lease not owned', {
          cause: err,
        });
      }
      throw err;
    }
  }

  async releaseLease(
    args: FaceTheoryIsrMetaStoreReleaseLeaseArgs,
  ): Promise<void> {
    const cacheKey = args.cacheKey;
    const pk = this.pkFromCacheKey(cacheKey);

    if (!args.leaseToken) throw new Error('leaseToken is required');

    const mgr = new LeaseManager(this.ddb, this.tableName, {
      ...(this.leaseToken ? { token: this.leaseToken } : {}),
      ttlBufferSeconds: this.leaseTtlBufferSeconds,
    });

    await mgr.release({
      key: { pk, sk: LOCK_SK },
      token: args.leaseToken,
      expiresAt: 0,
    });
  }
}

export function createFaceTheoryIsrMetaStore(
  cfg: FaceTheoryIsrMetaStoreConfig,
): FaceTheoryIsrMetaStore {
  return new FaceTheoryIsrMetaStore(cfg);
}
