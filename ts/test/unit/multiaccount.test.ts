import test from 'node:test';
import assert from 'node:assert/strict';

import type { AssumeRoleCommandOutput } from '@aws-sdk/client-sts';

import {
  MultiAccountDynamoDBClients,
  createAssumeRoleCredentialsProvider,
} from '../../src/multiaccount.js';

class FakeSTS {
  calls = 0;

  constructor(private readonly makeOutput: () => AssumeRoleCommandOutput) {}

  async send(): Promise<AssumeRoleCommandOutput> {
    this.calls += 1;
    return this.makeOutput();
  }
}

test('createAssumeRoleCredentialsProvider caches until refresh window', async () => {
  let nowMs = 0;
  const now = () => nowMs;

  const sts = new FakeSTS(() => ({
    $metadata: {},
    Credentials: {
      AccessKeyId: 'AKIA1',
      SecretAccessKey: 'SECRET1',
      SessionToken: 'TOKEN1',
      Expiration: new Date(1_000),
    },
  }));

  const provider = createAssumeRoleCredentialsProvider({
    roleArn: 'arn:aws:iam::111111111111:role/Test',
    externalId: 'ext',
    region: 'us-east-1',
    now,
    refreshBeforeMs: 200,
    sts: sts as never,
  });

  nowMs = 0;
  const c1 = await provider();
  assert.equal(c1.accessKeyId, 'AKIA1');
  assert.equal(sts.calls, 1);

  nowMs = 700; // 700 < (1000 - 200) => cached
  const c2 = await provider();
  assert.equal(c2.accessKeyId, 'AKIA1');
  assert.equal(sts.calls, 1);

  nowMs = 850; // 850 >= (1000 - 200) => refresh
  await provider();
  assert.equal(sts.calls, 2);
});

test('MultiAccountDynamoDBClients caches per partner id', () => {
  const mac = new MultiAccountDynamoDBClients(
    {
      p1: {
        roleArn: 'arn:aws:iam::111111111111:role/Test',
        region: 'us-east-1',
      },
    },
    {
      sts: new FakeSTS(() => ({
        $metadata: {},
        Credentials: {
          AccessKeyId: 'AKIA1',
          SecretAccessKey: 'SECRET1',
          SessionToken: 'TOKEN1',
          Expiration: new Date(1_000),
        },
      })) as never,
    },
  );

  const a = mac.client('p1');
  const b = mac.client('p1');
  assert.equal(a, b);

  assert.throws(() => mac.client('missing'));
});
