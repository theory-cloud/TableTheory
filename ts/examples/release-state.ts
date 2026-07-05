import {
  TransactWriteItemsCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient, defineModel } from '../src/index.js';
import {
  transitionReleaseState,
  validateDeployAuthorityMetadata,
} from '../src/release-state.js';

const tableName = 'release_state_contract';

const ReleaseStateActual = defineModel({
  name: 'ReleaseStateActual',
  table: { name: tableName },
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
    { attribute: 'service', type: 'S', optional: true },
    { attribute: 'status', type: 'S', optional: true },
    { attribute: 'pinnedReleaseId', type: 'S', optional: true },
    { attribute: 'previousReleaseId', type: 'S', optional: true },
    { attribute: 'provenance', type: 'M', optional: true, json: true },
    { attribute: 'confidence', type: 'M', optional: true, json: true },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

const ReleaseStateEvent = defineModel({
  name: 'ReleaseStateEvent',
  table: { name: tableName },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  write_policy: { mode: 'write_once' },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'service', type: 'S', optional: true },
    { attribute: 'releaseId', type: 'S', optional: true },
    { attribute: 'eventType', type: 'S', optional: true },
    { attribute: 'at', type: 'S', optional: true },
    { attribute: 'actor', type: 'S', optional: true },
    { attribute: 'evidence', type: 'M', optional: true, json: true },
  ],
});

const ReleaseStateOutbox = defineModel({
  name: 'ReleaseStateOutbox',
  table: { name: tableName },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  write_policy: { mode: 'write_once' },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'operation', type: 'S', optional: true },
    { attribute: 'idempotencyKey', type: 'S', optional: true },
    { attribute: 'requestedState', type: 'S', optional: true },
    { attribute: 'nextAttemptAt', type: 'S', optional: true },
  ],
});

class RecordingDdb {
  sent: unknown[] = [];

  async send(cmd: unknown): Promise<Record<string, never>> {
    this.sent.push(cmd);
    return {};
  }
}

const observedAt = '2026-04-24T19:00:00Z';
const recordedAt = '2026-04-24T19:00:01Z';
const service = 'service-a';
const releaseId = 'rel_002';
const ref = `operator://deploy/${service}/${releaseId}`;

const provenance = {
  mode: 'native',
  system: 'release-control-plane',
  kind: 'operator_command',
  ref,
  observed_at: observedAt,
  recorded_at: recordedAt,
  evidence: [
    {
      kind: 'operator_command',
      source: 'release-control-plane',
      ref,
      observed_at: observedAt,
    },
  ],
};

const confidence = {
  level: 'high',
  reasons: ['operator_command_authority'],
};

validateDeployAuthorityMetadata({ provenance, confidence });

const ddb = new RecordingDdb();
const client = new TheorydbClient(ddb as unknown as DynamoDBClient).register(
  ReleaseStateActual,
  ReleaseStateEvent,
  ReleaseStateOutbox,
);

await transitionReleaseState(client, {
  actualModel: 'ReleaseStateActual',
  actualKey: { PK: `RELEASE#${service}`, SK: 'ACTUAL' },
  expectedVersion: 7,
  set: {
    status: 'active',
    previousReleaseId: 'rel_001',
    provenance,
    confidence,
  },
  eventModel: 'ReleaseStateEvent',
  eventItem: {
    PK: `RELEASE#${service}`,
    SK: `EVENT#${observedAt}#${releaseId}`,
    service,
    releaseId,
    eventType: 'promoted',
    at: observedAt,
    actor: 'operator@example.com',
    evidence: provenance,
  },
});

const outbox = {
  PK: `RELEASE#${service}`,
  SK: `OUTBOX#lambda-alias#${releaseId}`,
  operation: 'lambda_alias_update',
  idempotencyKey: `${service}:${releaseId}`,
  requestedState: 'active',
  nextAttemptAt: observedAt,
};

const command = ddb.sent[0];
if (!(command instanceof TransactWriteItemsCommand)) {
  throw new Error('expected release-state helper to emit TransactWriteItems');
}

console.log(
  `transactionItems=${command.input.TransactItems?.length ?? 0} outbox=${outbox.SK}`,
);
