import assert from 'node:assert/strict';

import {
  TransactWriteItemsCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { TheorydbError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';
import {
  transitionReleaseState,
  validateDeployAuthorityMetadata,
} from '../../src/release-state.js';

const ReleaseStateActual = defineModel({
  name: 'ReleaseStateActual',
  table: { name: 'release_state_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  write_policy: {
    mode: 'mutable',
    protected_attributes: ['pinnedReleaseId'],
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'status', type: 'S', optional: true },
    { attribute: 'pinnedReleaseId', type: 'S', optional: true },
    { attribute: 'previousReleaseId', type: 'S', optional: true },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

const ReleaseStateEvent = defineModel({
  name: 'ReleaseStateEvent',
  table: { name: 'release_state_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  write_policy: {
    mode: 'write_once',
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'releaseId', type: 'S', optional: true },
    { attribute: 'eventType', type: 'S', optional: true },
  ],
});

const ReleaseStateOutbox = defineModel({
  name: 'ReleaseStateOutbox',
  table: { name: 'release_state_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  write_policy: {
    mode: 'write_once',
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'operation', type: 'S', optional: true },
    { attribute: 'idempotencyKey', type: 'S', optional: true },
  ],
});

class StubDdb {
  sent: unknown[] = [];

  async send(cmd: unknown): Promise<unknown> {
    this.sent.push(cmd);
    return {};
  }
}

{
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateActual,
    ReleaseStateEvent,
  );

  await transitionReleaseState(client, {
    actualModel: 'ReleaseStateActual',
    actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
    expectedVersion: 0,
    set: { status: 'active', previousReleaseId: 'rel_001' },
    eventModel: 'ReleaseStateEvent',
    eventItem: {
      PK: 'RELEASE#service-a',
      SK: 'EVENT#1',
      releaseId: 'rel_002',
      eventType: 'promoted',
    },
  });

  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof TransactWriteItemsCommand);
  assert.equal(cmd.input.TransactItems?.length, 2);

  const update = cmd.input.TransactItems?.[0]?.Update;
  assert.equal(update?.TableName, 'release_state_contract');
  assert.ok(update?.UpdateExpression?.includes('SET'));
  assert.ok(update?.UpdateExpression?.includes('ADD'));
  assert.ok(update?.ConditionExpression);
  assert.ok(
    Object.values(update?.ExpressionAttributeNames ?? {}).includes('version'),
  );

  const put = cmd.input.TransactItems?.[1]?.Put;
  assert.equal(put?.TableName, 'release_state_contract');
  assert.equal(put?.ConditionExpression, 'attribute_not_exists(#pk)');
}

{
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateActual,
    ReleaseStateEvent,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { version: 2 },
        eventModel: 'ReleaseStateEvent',
        eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
  assert.equal(ddb.sent.length, 0);
}

function validDeployAuthorityItem(): Record<string, unknown> {
  return {
    provenance: {
      mode: 'native',
      system: 'release-control-plane',
      kind: 'operator_command',
      ref: 'operator://deploy/service-a/rel_001',
      observed_at: '2026-04-24T19:00:00Z',
      recorded_at: '2026-04-24T19:00:01Z',
      evidence: [
        {
          kind: 'operator_command',
          source: 'release-control-plane',
          ref: 'operator://deploy/service-a/rel_001',
          observed_at: '2026-04-24T19:00:00Z',
        },
      ],
    },
    confidence: {
      level: 'high',
      reasons: ['operator_command_authority'],
    },
  };
}

{
  assert.doesNotThrow(() =>
    validateDeployAuthorityMetadata(validDeployAuthorityItem()),
  );
  assert.doesNotThrow(() => validateDeployAuthorityMetadata({ PK: 'A' }));
}

{
  const item = validDeployAuthorityItem();
  item.provenance = {
    mode: 'imported',
    system: 'partner-factory',
    kind: 'factory_batch_manifest',
    ref: 's3://factory/manifests/ambiguous.json',
    observed_at: '2026-04-24T19:10:00Z',
    recorded_at: '2026-04-24T19:10:01Z',
    evidence: [
      {
        kind: 'factory_batch_manifest',
        source: 'partner-factory',
        ref: 's3://factory/manifests/a.json',
        observed_at: '2026-04-24T19:09:59Z',
      },
      {
        kind: 'submodule_pin',
        source: 'service-ci',
        ref: 'https://github.com/acme/service-b/tree/conflicting',
        observed_at: '2026-04-24T19:09:59Z',
      },
    ],
  };
  item.confidence = { level: 'low', reasons: ['conflicting_evidence'] };

  assert.throws(
    () => validateDeployAuthorityMetadata(item),
    (e) =>
      e instanceof TheorydbError &&
      e.code === 'ErrRejectedDeployAuthorityEvidence',
  );
}

{
  const item = validDeployAuthorityItem();
  (item.provenance as Record<string, unknown>).notes = 'free form';
  assert.throws(
    () => validateDeployAuthorityMetadata(item),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
}

function provenanceOf(item: Record<string, unknown>): Record<string, unknown> {
  return item.provenance as Record<string, unknown>;
}

function evidenceOf(
  item: Record<string, unknown>,
): Array<Record<string, unknown>> {
  return provenanceOf(item).evidence as Array<Record<string, unknown>>;
}

{
  // An event may only describe the actual row it transitions.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateActual,
    ReleaseStateEvent,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { status: 'active' },
        eventModel: 'ReleaseStateEvent',
        eventItem: { PK: 'RELEASE#service-b', SK: 'EVENT#1' },
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // Actual update, event append, and conditional outbox create are one transaction.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateActual,
    ReleaseStateEvent,
    ReleaseStateOutbox,
  );

  await transitionReleaseState(client, {
    actualModel: 'ReleaseStateActual',
    actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
    set: { status: 'active' },
    eventModel: 'ReleaseStateEvent',
    eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
    outboxModel: 'ReleaseStateOutbox',
    outboxItem: { PK: 'RELEASE#service-a', SK: 'OUTBOX#1' },
  });

  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof TransactWriteItemsCommand);
  assert.equal(cmd.input.TransactItems?.length, 3);
  assert.equal(
    cmd.input.TransactItems?.[2]?.Put?.ConditionExpression,
    'attribute_not_exists(#pk)',
  );
}

{
  // A rejected transaction surfaces the failure and issues no second write.
  class FailingDdb {
    sent: unknown[] = [];
    async send(cmd: unknown): Promise<unknown> {
      this.sent.push(cmd);
      throw new Error('TransactionCanceledException');
    }
  }

  const ddb = new FailingDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateActual,
    ReleaseStateEvent,
    ReleaseStateOutbox,
  );

  await assert.rejects(() =>
    transitionReleaseState(client, {
      actualModel: 'ReleaseStateActual',
      actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
      set: { status: 'active' },
      eventModel: 'ReleaseStateEvent',
      eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
      outboxModel: 'ReleaseStateOutbox',
      outboxItem: { PK: 'RELEASE#service-a', SK: 'OUTBOX#1' },
    }),
  );
  assert.equal(ddb.sent.length, 1);
}

{
  // Same kind/source/ref with different digests conflicts.
  const item = validDeployAuthorityItem();
  const base = evidenceOf(item)[0]!;
  provenanceOf(item).evidence = [
    { ...base, digest: 'sha256:aaaaaaaa' },
    { ...base, digest: 'sha256:bbbbbbbb' },
  ];
  assert.throws(
    () => validateDeployAuthorityMetadata(item),
    (e) =>
      e instanceof TheorydbError &&
      e.code === 'ErrRejectedDeployAuthorityEvidence',
  );
}

{
  // Byte-for-byte identical evidence (including digest) stays idempotent.
  const item = validDeployAuthorityItem();
  const base = evidenceOf(item)[0]!;
  provenanceOf(item).evidence = [
    { ...base, digest: 'sha256:cccccccc' },
    { ...base, digest: 'sha256:cccccccc' },
  ];
  assert.doesNotThrow(() => validateDeployAuthorityMetadata(item));
}

{
  // Unrelated evidence may not bless a different top-level artifact.
  const item = validDeployAuthorityItem();
  provenanceOf(item).ref = 'operator://deploy/service-b/rel_999';
  assert.throws(
    () => validateDeployAuthorityMetadata(item),
    (e) =>
      e instanceof TheorydbError &&
      e.code === 'ErrRejectedDeployAuthorityEvidence',
  );
}

{
  // A top-level digest must align with the evidence digest.
  const item = validDeployAuthorityItem();
  const base = evidenceOf(item)[0]!;
  provenanceOf(item).evidence = [{ ...base, digest: 'sha256:dddddddd' }];
  provenanceOf(item).digest = 'sha256:dddddddd';
  assert.doesNotThrow(() => validateDeployAuthorityMetadata(item));

  provenanceOf(item).digest = 'sha256:eeeeeeee';
  assert.throws(
    () => validateDeployAuthorityMetadata(item),
    (e) =>
      e instanceof TheorydbError &&
      e.code === 'ErrRejectedDeployAuthorityEvidence',
  );
}
