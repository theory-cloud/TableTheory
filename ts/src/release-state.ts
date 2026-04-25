import type { TheorydbClient } from './client.js';
import { TheorydbError } from './errors.js';

const DEFAULT_VERSION_ATTRIBUTE = 'version';

/**
 * Input for a release-state registry transition. The actual-state update and
 * event-history append are committed through one DynamoDB transaction by the
 * supplied TableTheory client.
 *
 * External side effects such as Lambda alias flips or CodePipeline executions
 * are intentionally outside this helper's atomicity boundary. Callers should
 * pair those side effects with explicit retry/reconciliation/outbox behavior.
 */
export interface ReleaseStateTransitionInput {
  actualModel: string;
  actualKey: Record<string, unknown>;
  set: Record<string, unknown>;
  eventModel: string;
  eventItem: Record<string, unknown>;
  expectedVersion?: number;
  versionAttribute?: string;
}

/**
 * Transactionally update the release-state actual row and append one immutable
 * event-history row.
 */
export async function transitionReleaseState(
  client: TheorydbClient,
  input: ReleaseStateTransitionInput,
): Promise<void> {
  validateTransitionInput(client, input);
  const versionAttribute = input.versionAttribute ?? DEFAULT_VERSION_ATTRIBUTE;

  await client.transactWrite([
    {
      kind: 'update',
      model: input.actualModel,
      key: input.actualKey,
      updateFn: (builder) => {
        for (const [field, value] of Object.entries(input.set)) {
          builder.set(field, value);
        }
        builder.add(versionAttribute, 1);
        if (input.expectedVersion !== undefined) {
          builder.conditionVersion(input.expectedVersion);
        }
      },
    },
    {
      kind: 'put',
      model: input.eventModel,
      item: input.eventItem,
      ifNotExists: true,
    },
  ]);
}

function validateTransitionInput(
  client: TheorydbClient,
  input: ReleaseStateTransitionInput,
): void {
  if (!client) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'release-state client is required',
    );
  }
  if (!input.actualModel) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'release-state actualModel is required',
    );
  }
  if (!input.eventModel) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'release-state eventModel is required',
    );
  }
  if (Object.keys(input.actualKey).length === 0) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'release-state actualKey is required',
    );
  }
  if (Object.keys(input.eventItem).length === 0) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'release-state eventItem is required',
    );
  }
  if (Object.keys(input.set).length === 0) {
    throw new TheorydbError(
      'ErrInvalidOperator',
      'release-state transition set is required',
    );
  }

  const versionAttribute = input.versionAttribute ?? DEFAULT_VERSION_ATTRIBUTE;
  if (Object.hasOwn(input.set, versionAttribute)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'release-state transition set must not mutate version directly',
    );
  }
}
