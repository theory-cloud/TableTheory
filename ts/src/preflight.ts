import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';
import type { Model } from './model.js';

/**
 * ItemWriterPath names a TableTheory writer path whose stored DynamoDB
 * representation an item preflight must reproduce.
 *
 * Unlike Go, the TypeScript runtime serializes every whole item through one
 * function (`marshalPutItem`): `create`, `save`, `batchWrite` puts and
 * `transactWrite` puts all call it, so both paths currently return the same
 * representation. The two members exist so cross-runtime preflight code and the
 * shared nil/empty conformance table name writer paths identically on every
 * runtime, and so a future divergence stays expressible without an API change.
 */
export type ItemWriterPath = 'put' | 'transact_write';

/**
 * Returns the DynamoDB item that the named writer path stores for `item`.
 *
 * The returned item is byte-equal to the payload the real writer sends: for
 * `create`, `save`, `batchWrite` and `transactWrite` puts that is
 * `marshalPutItem`, which this function calls directly. Both writer paths are
 * pinned by regression tests that capture live payloads through the stateful
 * fake client, so neither answer can drift from its writer without failing the
 * build.
 *
 * `item` is the same plain object `create`/`save` would receive, and `opts.now`
 * overrides the lifecycle timestamp exactly as it does for the writers.
 *
 * Scope:
 *
 *   - Models that declare encrypted attributes are refused. An encrypted
 *     attribute is stored as a KMS-wrapped envelope that only the asynchronous
 *     writer path can produce, so a synchronous preflight cannot predict its
 *     bytes.
 *
 * An unrecognized path is refused. The preflight has no side effects and never
 * contacts DynamoDB.
 */
export function marshalItemForWriterPath(
  model: Model,
  item: Record<string, unknown>,
  path: ItemWriterPath,
  opts: { now?: string } = {},
): Record<string, AttributeValue> {
  throw new TheorydbError(
    'ErrInvalidModel',
    `marshalItemForWriterPath is not implemented (path: ${String(path)}, now: ${String(opts.now)}, model: ${model.name})`,
  );
}
