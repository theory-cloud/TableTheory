import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { modelHasEncryptedAttributes } from './encryption.js';
import { TheorydbError } from './errors.js';
import { marshalPutItem } from './marshal.js';
import type { Model } from './model.js';

const WRITER_PATHS: readonly ItemWriterPath[] = ['put', 'transact_write'];

/**
 * ItemWriterPath names a TableTheory writer path whose stored DynamoDB
 * representation an item preflight must reproduce.
 *
 * Unlike Go, the TypeScript runtime serializes every whole item through one
 * function (`marshalPutItem`): `create`, `save`, `batchWrite` puts and
 * `transactWrite` puts all call it, so both paths return the same
 * representation. That representation is a fixed compatibility contract, frozen
 * by the operator's policy that TableTheory supports its consumers rather than
 * changing stored data. The two members exist so cross-runtime preflight code and
 * the shared nil/empty conformance table name writer paths identically on every
 * runtime.
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
 *   - For a model that declares `createdAt` or `updatedAt`, the two writer paths
 *     stamp it differently: `create`, `save` and `batchWrite` pass the client
 *     clock to `marshalPutItem`, while the `transactWrite` put branch does not
 *     and therefore stamps the wall clock. Pass `opts.now` only when
 *     reproducing the path that honours it.
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
  if (!WRITER_PATHS.includes(path)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Unknown item writer path: ${String(path)}`,
    );
  }

  if (modelHasEncryptedAttributes(model)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Item preflight does not cover the encrypted attributes of model ${model.name}`,
    );
  }

  return marshalPutItem(model, item, opts);
}
