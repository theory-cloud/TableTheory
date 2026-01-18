import http from 'node:http';
import https from 'node:https';
import { performance } from 'node:perf_hooks';

import {
  DynamoDBClient,
  type DynamoDBClientConfig,
} from '@aws-sdk/client-dynamodb';
import { NodeHttpHandler } from '@smithy/node-http-handler';

import { TheorydbClient } from './client.js';

export type LambdaContextLike = {
  getRemainingTimeInMillis(): number;
};

export type LambdaMetric = {
  service: 'dynamodb';
  command: string;
  ms: number;
  ok: boolean;
};

export function isLambdaEnvironment(
  env: NodeJS.ProcessEnv = process.env,
): boolean {
  return Boolean(
    env.AWS_LAMBDA_FUNCTION_NAME ||
    env.AWS_EXECUTION_ENV?.includes('AWS_Lambda'),
  );
}

export function createLambdaTimeoutSignal(
  ctx: LambdaContextLike,
  opts: { bufferMs?: number } = {},
): { signal: AbortSignal; cleanup: () => void } {
  const bufferMs = opts.bufferMs ?? 1_000;
  const remainingMs = Math.max(0, ctx.getRemainingTimeInMillis() - bufferMs);

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), remainingMs);
  timer.unref?.();

  return {
    signal: controller.signal,
    cleanup: () => clearTimeout(timer),
  };
}

export function withLambdaTimeout(
  client: TheorydbClient,
  ctx: LambdaContextLike,
  opts: { bufferMs?: number } = {},
): { client: TheorydbClient; cleanup: () => void } {
  const { signal, cleanup } = createLambdaTimeoutSignal(ctx, opts);
  return { client: client.withSendOptions({ abortSignal: signal }), cleanup };
}

export function createLambdaDynamoDBClient(
  opts: DynamoDBClientConfig & {
    connectionTimeoutMs?: number;
    socketTimeoutMs?: number;
    metrics?: (m: LambdaMetric) => void;
  } = {},
): DynamoDBClient {
  const {
    connectionTimeoutMs = 1_000,
    socketTimeoutMs = 3_000,
    metrics,
    ...config
  } = opts;

  const httpAgent = new http.Agent({
    keepAlive: true,
    maxSockets: 50,
    maxFreeSockets: 50,
  });
  const httpsAgent = new https.Agent({
    keepAlive: true,
    maxSockets: 50,
    maxFreeSockets: 50,
  });

  const ddb = new DynamoDBClient({
    ...config,
    requestHandler: new NodeHttpHandler({
      connectionTimeout: connectionTimeoutMs,
      socketTimeout: socketTimeoutMs,
      httpAgent,
      httpsAgent,
    }),
  });

  if (metrics) {
    ddb.middlewareStack.add(
      (next, context) => async (args) => {
        const start = performance.now();
        try {
          const out = await next(args);
          metrics({
            service: 'dynamodb',
            command: context.commandName ?? 'UnknownCommand',
            ms: performance.now() - start,
            ok: true,
          });
          return out;
        } catch (err) {
          metrics({
            service: 'dynamodb',
            command: context.commandName ?? 'UnknownCommand',
            ms: performance.now() - start,
            ok: false,
          });
          throw err;
        }
      },
      { step: 'finalizeRequest', name: 'theorydbLambdaMetrics' },
    );
  }

  return ddb;
}

let globalLambdaDdb: DynamoDBClient | undefined;

export function getLambdaDynamoDBClient(
  opts: Parameters<typeof createLambdaDynamoDBClient>[0] = {},
): DynamoDBClient {
  globalLambdaDdb ??= createLambdaDynamoDBClient(opts);
  return globalLambdaDdb;
}
