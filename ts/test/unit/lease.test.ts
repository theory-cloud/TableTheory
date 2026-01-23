import assert from 'node:assert/strict';

import {
  ConditionalCheckFailedException,
  DeleteItemCommand,
  PutItemCommand,
  UpdateItemCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from '../../src/errors.js';
import { LeaseManager } from '../../src/lease.js';

class StubDdb {
  sent: unknown[] = [];

  constructor(private readonly handler: (cmd: unknown) => unknown) {}

  async send(cmd: unknown): Promise<unknown> {
    this.sent.push(cmd);
    return this.handler(cmd);
  }
}

{
  const ddb = new StubDdb(() => ({}));
  const mgr = new LeaseManager(ddb as unknown as DynamoDBClient, 'tbl', {
    now: () => 1000,
    token: () => 'tok',
    ttlBufferSeconds: 10,
  });

  const lease = await mgr.acquire(
    { pk: 'CACHE#A', sk: 'LOCK' },
    { leaseSeconds: 30 },
  );
  assert.equal(lease.token, 'tok');
  assert.equal(lease.expiresAt, 1030);

  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof PutItemCommand);
  assert.equal(cmd.input.TableName, 'tbl');
  assert.equal(
    cmd.input.ConditionExpression,
    'attribute_not_exists(#pk) OR #exp <= :now',
  );
  assert.deepEqual(cmd.input.ExpressionAttributeNames, {
    '#pk': 'pk',
    '#exp': 'lease_expires_at',
  });
  assert.equal(cmd.input.ExpressionAttributeValues?.[':now']?.N, '1000');
  assert.equal(cmd.input.Item?.pk?.S, 'CACHE#A');
  assert.equal(cmd.input.Item?.sk?.S, 'LOCK');
  assert.equal(cmd.input.Item?.lease_token?.S, 'tok');
  assert.equal(cmd.input.Item?.lease_expires_at?.N, '1030');
  assert.equal(cmd.input.Item?.ttl?.N, '1040');
}

{
  const cfe = new ConditionalCheckFailedException({
    $metadata: {},
    message: 'no',
  });
  const ddb = new StubDdb(() => {
    throw cfe;
  });
  const mgr = new LeaseManager(ddb as unknown as DynamoDBClient, 'tbl', {
    now: () => 1000,
    token: () => 'tok',
  });

  await assert.rejects(
    () => mgr.acquire({ pk: 'CACHE#A', sk: 'LOCK' }, { leaseSeconds: 30 }),
    (e) => e instanceof TheorydbError && e.code === 'ErrLeaseHeld',
  );
}

{
  const ddb = new StubDdb(() => ({}));
  const mgr = new LeaseManager(ddb as unknown as DynamoDBClient, 'tbl', {
    now: () => 1000,
    ttlBufferSeconds: 10,
  });

  const lease = await mgr.refresh(
    { key: { pk: 'CACHE#A', sk: 'LOCK' }, token: 'tok', expiresAt: 0 },
    { leaseSeconds: 60 },
  );
  assert.equal(lease.expiresAt, 1060);

  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof UpdateItemCommand);
  assert.equal(cmd.input.TableName, 'tbl');
  assert.equal(cmd.input.ConditionExpression, '#tok = :tok AND #exp > :now');
  assert.equal(cmd.input.ExpressionAttributeValues?.[':tok']?.S, 'tok');
  assert.equal(cmd.input.ExpressionAttributeValues?.[':now']?.N, '1000');
  assert.equal(cmd.input.ExpressionAttributeValues?.[':exp']?.N, '1060');
  assert.equal(cmd.input.ExpressionAttributeValues?.[':ttl']?.N, '1070');
}

{
  const cfe = new ConditionalCheckFailedException({
    $metadata: {},
    message: 'no',
  });
  const ddb = new StubDdb(() => {
    throw cfe;
  });
  const mgr = new LeaseManager(ddb as unknown as DynamoDBClient, 'tbl', {
    now: () => 1000,
  });

  await assert.rejects(
    () =>
      mgr.refresh(
        { key: { pk: 'CACHE#A', sk: 'LOCK' }, token: 'tok', expiresAt: 0 },
        { leaseSeconds: 60 },
      ),
    (e) => e instanceof TheorydbError && e.code === 'ErrLeaseNotOwned',
  );
}

{
  const cfe = new ConditionalCheckFailedException({
    $metadata: {},
    message: 'no',
  });
  const ddb = new StubDdb(() => {
    throw cfe;
  });
  const mgr = new LeaseManager(ddb as unknown as DynamoDBClient, 'tbl');

  await mgr.release({
    key: { pk: 'CACHE#A', sk: 'LOCK' },
    token: 'tok',
    expiresAt: 0,
  });
  assert.ok(ddb.sent[0] instanceof DeleteItemCommand);
}
