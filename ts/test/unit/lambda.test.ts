import test from 'node:test';
import assert from 'node:assert/strict';
import { getActiveResourcesInfo } from 'node:process';

import * as lambdaModule from '../../src/lambda.js';
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

void test('isLambdaEnvironment detects lambda env vars', () => {
  assert.equal(isLambdaEnvironment({}), false);
  assert.equal(isLambdaEnvironment({ AWS_LAMBDA_FUNCTION_NAME: 'fn' }), true);
  assert.equal(
    isLambdaEnvironment({ AWS_EXECUTION_ENV: 'AWS_Lambda_nodejs24.x' }),
    true,
  );
});

void test('createLambdaTimeoutSignal aborts and supports cleanup', async () => {
  {
    const { signal } = createLambdaTimeoutSignal(
      { getRemainingTimeInMillis: () => 0 },
      { bufferMs: 0 },
    );
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

void test('createLambdaTimeoutSignal applies default and custom buffers', async () => {
  {
    const { signal } = createLambdaTimeoutSignal({
      getRemainingTimeInMillis: () => DEFAULT_LAMBDA_TIMEOUT_BUFFER_MS,
    });
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

void test('createLambdaDynamoDBClient and getLambdaDynamoDBClient build clients', () => {
  createLambdaDynamoDBClient({ region: 'us-east-1' });
  createLambdaDynamoDBClient({ region: 'us-east-1', metrics: () => {} });

  const a = getLambdaDynamoDBClient({ region: 'us-east-1' });
  const b = getLambdaDynamoDBClient({ region: 'us-east-1' });
  assert.equal(a, b);
});

void test('withLambdaTimeout returns a derived TheorydbClient', async () => {
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
  assert.equal(sendOptions[0]?.abortSignal?.aborted, true);
  cleanup();
});

function countActiveTimeouts(): number {
  return getActiveResourcesInfo().filter((kind) => kind === 'Timeout').length;
}

void test('createLambdaTimeoutSignal does not keep the invocation open', () => {
  // The watchdog timer is per-invocation. It must not hold the event loop open:
  // Lambda freezes the execution environment as soon as the handler returns, so
  // a live handle here would mean work outliving the invocation that started it.
  const before = countActiveTimeouts();

  const { cleanup } = createLambdaTimeoutSignal({
    getRemainingTimeInMillis: () => 60_000,
  });
  assert.equal(
    countActiveTimeouts(),
    before,
    'the timeout watchdog must be unref-ed so it cannot outlive the invocation',
  );

  cleanup();
  assert.equal(countActiveTimeouts(), before);
});

void test('lambda helpers expose no cold-start pre-warm', () => {
  // TableTheory's Lambda surface is synchronous: there is deliberately no
  // pre-warm that runs detached from the init that started it. Adding one here
  // would need the same treatment as Go's removed pre-warm, so this test fails
  // on purpose to force that review.
  const preWarmName = /pre[-_]?warm|warm[-_]?up|cold[-_]?start|optimi[sz]e/i;
  const offenders = Object.keys(lambdaModule).filter((name) =>
    preWarmName.test(name),
  );
  assert.deepEqual(offenders, []);
});

void test('getLambdaDynamoDBClient starts no detached work', () => {
  const before = countActiveTimeouts();

  const first = getLambdaDynamoDBClient({ region: 'us-east-1' });
  const second = getLambdaDynamoDBClient({ region: 'us-east-1' });

  assert.equal(
    first,
    second,
    'client construction must stay synchronous and cached',
  );
  assert.equal(countActiveTimeouts(), before);
});
