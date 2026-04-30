import test from 'node:test';
import assert from 'node:assert/strict';

import {
  DEFAULT_LAMBDA_TIMEOUT_BUFFER_MS,
  createLambdaTimeoutSignal,
  createLambdaDynamoDBClient,
  getLambdaDynamoDBClient,
  isLambdaEnvironment,
  withLambdaTimeout,
} from '../../src/lambda.js';
import { TheorydbClient } from '../../src/client.js';
import { defineModel } from '../../src/model.js';
import { DynamoDBClient, PutItemCommand } from '@aws-sdk/client-dynamodb';
import type { SendOptions } from '../../src/send-options.js';

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

test('createLambdaTimeoutSignal applies default and custom buffers', async () => {
  {
    const { signal } = createLambdaTimeoutSignal({
      getRemainingTimeInMillis: () => DEFAULT_LAMBDA_TIMEOUT_BUFFER_MS,
    });
    await new Promise((r) => setTimeout(r, 0));
    assert.equal(signal.aborted, true);
  }

  {
    const { signal, cleanup } = createLambdaTimeoutSignal(
      { getRemainingTimeInMillis: () => 20 },
      { bufferMs: 5 },
    );
    await new Promise((r) => setTimeout(r, 0));
    assert.equal(signal.aborted, false);
    cleanup();
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
  const sendOptions: (SendOptions | undefined)[] = [];
  const ddb = {
    send: async (
      command: PutItemCommand,
      options?: SendOptions,
    ): Promise<unknown> => {
      assert.equal(command instanceof PutItemCommand, true);
      sendOptions.push(options);
      return { $metadata: {} };
    },
  } as unknown as DynamoDBClient;

  const model = defineModel({
    name: 'T',
    table: { name: 't' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [{ attribute: 'PK', type: 'S', roles: ['pk'] }],
  });

  const base = new TheorydbClient(ddb).register(model);
  const { client, cleanup } = withLambdaTimeout(
    base,
    { getRemainingTimeInMillis: () => 0 },
    { bufferMs: 0 },
  );
  await client.create('T', { PK: 'A' });
  assert.equal(sendOptions.length, 1);
  assert.equal(sendOptions[0]?.abortSignal instanceof AbortSignal, true);
  cleanup();
});
