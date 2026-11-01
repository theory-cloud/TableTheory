import {
  CreateTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
  ResourceInUseException,
} from '@aws-sdk/client-dynamodb';

import { TheorydbClient, defineModel } from '../src/index.js';
import {
  transitionReleaseState,
  validateDeployAuthorityMetadata,
} from '../src/release-state.js';

// The runtime-specific table keeps the Go, TypeScript and Python rows from
// sharing seed state in one harness database; their assertions and printed
// parity line are identical.
const tableName = 'release_state_contract_ts';
const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';
const region = process.env.AWS_REGION ?? 'us-east-1';

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

const ddb = new DynamoDBClient({
  region,
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

await ensureReleaseStateTable(ddb);

const db = new TheorydbClient(ddb).register(
  ReleaseStateActual,
  ReleaseStateEvent,
  ReleaseStateOutbox,
);

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

const actualKey = { PK: `RELEASE#${service}`, SK: 'ACTUAL' };
const eventKey = {
  PK: `RELEASE#${service}`,
  SK: `EVENT#${observedAt}#${releaseId}`,
};
const outboxKey = {
  PK: `RELEASE#${service}`,
  SK: `OUTBOX#lambda-alias#${releaseId}`,
};

await db.create(
  'ReleaseStateActual',
  {
    ...actualKey,
    service,
    status: 'idle',
    version: 7,
  },
  { ifNotExists: true },
);

const transition = {
  actualModel: 'ReleaseStateActual',
  actualKey,
  expectedVersion: 7,
  set: {
    status: 'active',
    previousReleaseId: 'rel_001',
    provenance,
    confidence,
  },
  eventModel: 'ReleaseStateEvent',
  eventItem: {
    ...eventKey,
    service,
    releaseId,
    eventType: 'promoted',
    at: observedAt,
    actor: 'operator@example.com',
    evidence: provenance,
  },
  outboxModel: 'ReleaseStateOutbox',
  outboxItem: {
    ...outboxKey,
    operation: 'lambda_alias_update',
    idempotencyKey: `${service}:${releaseId}`,
    requestedState: 'active',
    nextAttemptAt: observedAt,
  },
};

await transitionReleaseState(db, transition);

const actual = await db.get('ReleaseStateActual', actualKey);
const version = Number(actual.version);
if (actual.status !== 'active' || actual.previousReleaseId !== 'rel_001') {
  throw new Error(
    `actual row status=${String(actual.status)} previousReleaseId=${String(
      actual.previousReleaseId,
    )}; want status=active previousReleaseId=rel_001`,
  );
}
if (version !== 8) {
  throw new Error(`actual version=${version}; want 8`);
}

const event = await db.get('ReleaseStateEvent', eventKey);
if (event.eventType !== 'promoted' || event.releaseId !== releaseId) {
  throw new Error(
    `event row eventType=${String(event.eventType)} releaseId=${String(
      event.releaseId,
    )}; want promoted/${releaseId}`,
  );
}

const outbox = await db.get('ReleaseStateOutbox', outboxKey);
if (outbox.operation !== 'lambda_alias_update') {
  throw new Error(
    `outbox operation=${String(outbox.operation)}; want lambda_alias_update`,
  );
}

let staleRejected = false;
try {
  await transitionReleaseState(db, transition);
} catch {
  staleRejected = true;
}
if (!staleRejected) {
  throw new Error(
    'stale expectedVersion transition succeeded; want a condition failure',
  );
}

console.log(
  `release-state: actual=${actualKey.PK}/${actualKey.SK} version=${version} status=${String(
    actual.status,
  )} event=${eventKey.SK} outbox=${outboxKey.SK}`,
);

ddb.destroy();

async function ensureReleaseStateTable(
  client: DynamoDBClient,
): Promise<void> {
  try {
    await client.send(new DescribeTableCommand({ TableName: tableName }));
    return;
  } catch {
    // continue
  }

  try {
    await client.send(
      new CreateTableCommand({
        TableName: tableName,
        AttributeDefinitions: [
          { AttributeName: 'PK', AttributeType: 'S' },
          { AttributeName: 'SK', AttributeType: 'S' },
        ],
        KeySchema: [
          { AttributeName: 'PK', KeyType: 'HASH' },
          { AttributeName: 'SK', KeyType: 'RANGE' },
        ],
        BillingMode: 'PAY_PER_REQUEST',
      }),
    );
  } catch (err) {
    if (err instanceof ResourceInUseException) return;
    if (
      typeof err === 'object' &&
      err !== null &&
      'name' in err &&
      (err as { name?: unknown }).name === 'ResourceInUseException'
    ) {
      return;
    }
    throw err;
  }

  for (let attempt = 0; attempt < 25; attempt += 1) {
    const described = await client.send(
      new DescribeTableCommand({ TableName: tableName }),
    );
    if (described.Table?.TableStatus === 'ACTIVE') return;
    await new Promise((resolve) => {
      setTimeout(resolve, 200);
    });
  }
}
