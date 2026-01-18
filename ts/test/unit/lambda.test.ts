import test from 'node:test';
import assert from 'node:assert/strict';

import {
  createLambdaTimeoutSignal,
  createLambdaDynamoDBClient,
  getLambdaDynamoDBClient,
  isLambdaEnvironment,
  withLambdaTimeout,
} from '../../src/lambda.js';
import { TheorydbClient } from '../../src/client.js';
import { defineModel } from '../../src/model.js';
import { createMockDynamoDBClient } from '../../src/testkit/index.js';
import { PutItemCommand } from '@aws-sdk/client-dynamodb';

test('isLambdaEnvironment detects lambda env vars', () => {
  assert.equal(isLambdaEnvironment({}), false);
  assert.equal(isLambdaEnvironment({ AWS_LAMBDA_FUNCTION_NAME: 'fn' }), true);
  assert.equal(
    isLambdaEnvironment({ AWS_EXECUTION_ENV: 'AWS_Lambda_nodejs24.x' }),
    true,
  );
});

test('createLambdaTimeoutSignal aborts and supports cleanup', async () => {
  {
    const { signal } = createLambdaTimeoutSignal(
      { getRemainingTimeInMillis: () => 0 },
      { bufferMs: 0 },
    );
    await new Promise((r) => setTimeout(r, 0));
    assert.equal(signal.aborted, true);
  }

  {
    const { signal, cleanup } = createLambdaTimeoutSignal(
      { getRemainingTimeInMillis: () => 10 },
      { bufferMs: 0 },
    );
    cleanup();
    await new Promise((r) => setTimeout(r, 20));
    assert.equal(signal.aborted, false);
  }
});

test('createLambdaDynamoDBClient and getLambdaDynamoDBClient build clients', () => {
  createLambdaDynamoDBClient({ region: 'us-east-1' });
  createLambdaDynamoDBClient({ region: 'us-east-1', metrics: () => {} });

  const a = getLambdaDynamoDBClient({ region: 'us-east-1' });
  const b = getLambdaDynamoDBClient({ region: 'us-east-1' });
  assert.equal(a, b);
});

test('withLambdaTimeout returns a derived TheorydbClient', async () => {
  const mock = createMockDynamoDBClient();
  mock.when(PutItemCommand, async () => ({ $metadata: {} }));

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [{ attribute: 'PK', type: 'S', roles: ['pk'] }],
  });

  const base = new TheorydbClient(mock.client).register(model);
  const { client, cleanup } = withLambdaTimeout(
    base,
    { getRemainingTimeInMillis: () => 0 },
    { bufferMs: 0 },
  );
  await client.create('T', { PK: 'A' });
  cleanup();
});
