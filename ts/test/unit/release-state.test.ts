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

const ReleaseStateBindingActual = defineModel({
  name: 'ReleaseStateBindingActual',
  table: { name: 'release_state_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  write_policy: {
    mode: 'mutable',
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'pinnedReleaseId', type: 'S', optional: true },
    { attribute: 'status', type: 'S', optional: true },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

const ReleaseStateBindingEvent = defineModel({
  name: 'ReleaseStateBindingEvent',
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
    { attribute: 'version', type: 'N', optional: true },
  ],
});

const ReleaseStateOutboxByRelease = defineModel({
  name: 'ReleaseStateOutboxByRelease',
  table: { name: 'release_state_outbox_by_release' },
  keys: {
    partition: { attribute: 'ReleasePK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  write_policy: {
    mode: 'write_once',
  },
  attributes: [
    { attribute: 'ReleasePK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'operation', type: 'S', optional: true },
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
        expectedVersion: 1,
        set: { status: 'active' },
        eventModel: 'ReleaseStateEvent',
        eventItem: {
          PK: 'RELEASE#service-a',
          SK: 'EVENT#2',
          eventType: 'promoted',
        },
        versionAttribute: 'otherCounter',
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // A declared event binding ties the event row to the authorizing transition.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
  );

  await transitionReleaseState(client, {
    actualModel: 'ReleaseStateBindingActual',
    actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
    set: { pinnedReleaseId: 'rel_002' },
    eventModel: 'ReleaseStateBindingEvent',
    eventItem: {
      PK: 'RELEASE#service-a',
      SK: 'EVENT#1',
      releaseId: 'rel_002',
    },
    eventBinding: {
      releaseId: 'set:pinnedReleaseId',
      PK: 'actual:PK',
    },
  });

  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof TransactWriteItemsCommand);
  assert.equal(cmd.input.TransactItems?.length, 2);
}

{
  // An event that disagrees with the transition value is rejected.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateBindingActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { pinnedReleaseId: 'rel_002' },
        eventModel: 'ReleaseStateBindingEvent',
        eventItem: {
          PK: 'RELEASE#service-a',
          SK: 'EVENT#1',
          releaseId: 'rel_001',
        },
        eventBinding: { releaseId: 'set:pinnedReleaseId' },
      }),
    (e) =>
      e instanceof TheorydbError &&
      e.code === 'ErrInvalidModel' &&
      e.message.includes('releaseId') &&
      e.message.includes('event'),
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // A bound attribute missing from the event row is rejected.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateBindingActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { pinnedReleaseId: 'rel_002' },
        eventModel: 'ReleaseStateBindingEvent',
        eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
        eventBinding: { releaseId: 'set:pinnedReleaseId' },
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // A declared outbox binding must agree with the transition.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
    ReleaseStateOutbox,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateBindingActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { pinnedReleaseId: 'rel_002' },
        eventModel: 'ReleaseStateBindingEvent',
        eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
        outboxModel: 'ReleaseStateOutbox',
        outboxItem: {
          PK: 'RELEASE#service-a',
          SK: 'OUTBOX#1',
          operation: 'rel_001',
        },
        outboxBinding: { operation: 'set:pinnedReleaseId' },
      }),
    (e) =>
      e instanceof TheorydbError &&
      e.code === 'ErrInvalidModel' &&
      e.message.includes('operation') &&
      e.message.includes('outbox'),
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // Outbox binding without an outbox row is rejected.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateBindingActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { pinnedReleaseId: 'rel_002' },
        eventModel: 'ReleaseStateBindingEvent',
        eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
        outboxBinding: { operation: 'set:pinnedReleaseId' },
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // Same-name outbox partition keys must identify the same release.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
    ReleaseStateOutbox,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateBindingActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { pinnedReleaseId: 'rel_002' },
        eventModel: 'ReleaseStateBindingEvent',
        eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
        outboxModel: 'ReleaseStateOutbox',
        outboxItem: { PK: 'RELEASE#service-b', SK: 'OUTBOX#1' },
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // A differently-keyed outbox model is legitimate and needs no shared PK.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
    ReleaseStateOutboxByRelease,
  );

  await transitionReleaseState(client, {
    actualModel: 'ReleaseStateBindingActual',
    actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
    set: { pinnedReleaseId: 'rel_002' },
    eventModel: 'ReleaseStateBindingEvent',
    eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1' },
    outboxModel: 'ReleaseStateOutboxByRelease',
    outboxItem: { ReleasePK: 'RELEASE#service-b', SK: 'OUTBOX#1' },
  });

  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof TransactWriteItemsCommand);
  assert.equal(cmd.input.TransactItems?.length, 3);
}

{
  // nextVersion binds the event to expectedVersion + 1 (numeric string allowed).
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
  );

  await transitionReleaseState(client, {
    actualModel: 'ReleaseStateBindingActual',
    actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
    expectedVersion: 3,
    set: { pinnedReleaseId: 'rel_002' },
    eventModel: 'ReleaseStateBindingEvent',
    eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1', version: '4' },
    eventBinding: { version: 'nextVersion' },
  });

  assert.equal(ddb.sent.length, 1);
}

{
  // An event version other than expectedVersion + 1 is rejected.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateBindingActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        expectedVersion: 3,
        set: { pinnedReleaseId: 'rel_002' },
        eventModel: 'ReleaseStateBindingEvent',
        eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1', version: 9 },
        eventBinding: { version: 'nextVersion' },
      }),
    (e) =>
      e instanceof TheorydbError &&
      e.code === 'ErrInvalidModel' &&
      e.message.includes('version'),
  );
  assert.equal(ddb.sent.length, 0);
}

{
  // nextVersion requires an expectedVersion.
  const ddb = new StubDdb();
  const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
    ReleaseStateBindingActual,
    ReleaseStateBindingEvent,
  );

  await assert.rejects(
    () =>
      transitionReleaseState(client, {
        actualModel: 'ReleaseStateBindingActual',
        actualKey: { PK: 'RELEASE#service-a', SK: 'ACTUAL' },
        set: { pinnedReleaseId: 'rel_002' },
        eventModel: 'ReleaseStateBindingEvent',
        eventItem: { PK: 'RELEASE#service-a', SK: 'EVENT#1', version: 1 },
        eventBinding: { version: 'nextVersion' },
      }),
    (e) => e instanceof TheorydbError && e.code === 'ErrInvalidModel',
  );
  assert.equal(ddb.sent.length, 0);
}
