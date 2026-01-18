import {
  DynamoDBClient,
  type DynamoDBClientConfig,
} from '@aws-sdk/client-dynamodb';
import {
  AssumeRoleCommand,
  STSClient,
  type AssumeRoleCommandOutput,
} from '@aws-sdk/client-sts';
import type {
  AwsCredentialIdentity,
  AwsCredentialIdentityProvider,
} from '@aws-sdk/types';

export type StsClientLike = {
  send(cmd: AssumeRoleCommand): Promise<AssumeRoleCommandOutput>;
};

export type AssumeRoleCredentialsProviderOptions = {
  roleArn: string;
  externalId?: string;
  region?: string;
  durationSeconds?: number;
  sessionName?: string;
  refreshBeforeMs?: number;
  now?: () => number;
  sts?: StsClientLike;
};

export function createAssumeRoleCredentialsProvider(
  opts: AssumeRoleCredentialsProviderOptions,
): AwsCredentialIdentityProvider {
  const now = opts.now ?? (() => Date.now());
  const refreshBeforeMs = opts.refreshBeforeMs ?? 5 * 60 * 1000;
  const durationSeconds = opts.durationSeconds ?? 3600;

  const sts: StsClientLike =
    opts.sts ?? new STSClient({ region: opts.region ?? 'us-east-1' });

  let cached: { creds: AwsCredentialIdentity; expiresAtMs: number } | undefined;

  return async () => {
    const current = cached;
    if (current && now() < current.expiresAtMs - refreshBeforeMs)
      return current.creds;

    const resp = await sts.send(
      new AssumeRoleCommand({
        RoleArn: opts.roleArn,
        RoleSessionName: opts.sessionName ?? 'theorydb',
        DurationSeconds: durationSeconds,
        ...(opts.externalId !== undefined
          ? { ExternalId: opts.externalId }
          : {}),
      }),
    );

    const c = resp.Credentials;
    if (!c?.AccessKeyId || !c.SecretAccessKey) {
      throw new Error('AssumeRole did not return credentials');
    }

    const expiresAtMs =
      c.Expiration instanceof Date
        ? c.Expiration.getTime()
        : now() + durationSeconds * 1000;

    const creds: AwsCredentialIdentity = {
      accessKeyId: c.AccessKeyId,
      secretAccessKey: c.SecretAccessKey,
      ...(c.SessionToken ? { sessionToken: c.SessionToken } : {}),
    };

    cached = { creds, expiresAtMs };
    return creds;
  };
}

export type AssumeRoleDynamoDBClientOptions = {
  roleArn: string;
  externalId?: string;
  region: string;
  durationSeconds?: number;
  sessionName?: string;
  refreshBeforeMs?: number;
  now?: () => number;
  sts?: StsClientLike;
  dynamo?: Omit<DynamoDBClientConfig, 'credentials' | 'region'>;
};

export function createAssumeRoleDynamoDBClient(
  opts: AssumeRoleDynamoDBClientOptions,
): DynamoDBClient {
  const credentials = createAssumeRoleCredentialsProvider({
    roleArn: opts.roleArn,
    region: opts.region,
    ...(opts.externalId !== undefined ? { externalId: opts.externalId } : {}),
    ...(opts.durationSeconds !== undefined
      ? { durationSeconds: opts.durationSeconds }
      : {}),
    ...(opts.sessionName !== undefined
      ? { sessionName: opts.sessionName }
      : {}),
    ...(opts.refreshBeforeMs !== undefined
      ? { refreshBeforeMs: opts.refreshBeforeMs }
      : {}),
    ...(opts.now !== undefined ? { now: opts.now } : {}),
    ...(opts.sts !== undefined ? { sts: opts.sts } : {}),
  });

  return new DynamoDBClient({
    region: opts.region,
    credentials,
    ...(opts.dynamo ?? {}),
  });
}

export type AccountConfig = {
  roleArn: string;
  externalId?: string;
  region: string;
  durationSeconds?: number;
};

export class MultiAccountDynamoDBClients {
  private readonly accounts: Record<string, AccountConfig>;
  private readonly clients = new Map<string, DynamoDBClient>();

  constructor(
    accounts: Record<string, AccountConfig>,
    private readonly opts: Omit<
      AssumeRoleDynamoDBClientOptions,
      'roleArn' | 'externalId' | 'region'
    > & {
      dynamo?: Omit<DynamoDBClientConfig, 'credentials' | 'region'>;
    } = {},
  ) {
    this.accounts = { ...accounts };
  }

  client(partnerId: string): DynamoDBClient {
    const existing = this.clients.get(partnerId);
    if (existing) return existing;

    const account = this.accounts[partnerId];
    if (!account) {
      throw new Error(`unknown partner: ${partnerId}`);
    }

    const client = createAssumeRoleDynamoDBClient({
      roleArn: account.roleArn,
      region: account.region,
      ...(account.externalId !== undefined
        ? { externalId: account.externalId }
        : {}),
      ...(account.durationSeconds !== undefined
        ? { durationSeconds: account.durationSeconds }
        : {}),
      ...this.opts,
    });

    this.clients.set(partnerId, client);
    return client;
  }
}
