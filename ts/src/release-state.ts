import type { TheorydbClient } from './client.js';
import { TheorydbError } from './errors.js';

const DEFAULT_VERSION_ATTRIBUTE = 'version';
const RFC3339_RE =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;
const PROVENANCE_KEYS = new Set([
  'mode',
  'system',
  'kind',
  'ref',
  'commit_sha',
  'observed_at',
  'recorded_at',
  'import_run_id',
  'evidence',
]);
const EVIDENCE_KEYS = new Set([
  'kind',
  'source',
  'ref',
  'observed_at',
  'digest',
]);
const CONFIDENCE_KEYS = new Set(['level', 'reasons']);
const PROVENANCE_MODES = new Set(['native', 'imported', 'inferred']);

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

/**
 * Validate deterministic provenance/confidence metadata for a
 * deploy-authoritative release-state actual row.
 *
 * Ambiguous/conflicting or low-confidence evidence is rejected so it cannot be
 * persisted as deploy authority. Preserve that evidence as separate immutable
 * visibility/event records instead.
 */
export function validateDeployAuthorityMetadata(
  item: Record<string, unknown>,
): void {
  const hasProvenance = Object.hasOwn(item, 'provenance');
  const hasConfidence = Object.hasOwn(item, 'confidence');
  if (!hasProvenance && !hasConfidence) return;
  if (!hasProvenance || !hasConfidence) {
    invalidModel('provenance and confidence must be provided together');
  }

  const provenance = recordValue(item.provenance, 'provenance');
  const confidence = recordValue(item.confidence, 'confidence');
  validateAllowedKeys('provenance', provenance, PROVENANCE_KEYS);
  validateAllowedKeys('confidence', confidence, CONFIDENCE_KEYS);
  validateProvenanceShape(provenance);

  const expectedReason = deriveDeployAuthorityReason(provenance);
  validateConfidence(confidence, expectedReason);
}

function validateProvenanceShape(provenance: Record<string, unknown>): void {
  const mode = requiredString(provenance, 'mode');
  if (!PROVENANCE_MODES.has(mode)) {
    invalidModel(`unsupported provenance.mode: ${mode}`);
  }
  for (const key of ['system', 'kind', 'ref']) {
    requiredString(provenance, key);
  }
  for (const key of ['observed_at', 'recorded_at']) {
    validateRfc3339(key, requiredString(provenance, key));
  }
  for (const key of ['commit_sha', 'import_run_id']) {
    if (Object.hasOwn(provenance, key) && typeof provenance[key] !== 'string') {
      invalidModel(`provenance.${key} must be a string`);
    }
  }
}

function deriveDeployAuthorityReason(
  provenance: Record<string, unknown>,
): string {
  const evidence = evidenceValues(provenance.evidence);
  if (evidence.length === 0) {
    rejected('deploy authority requires evidence');
  }

  let signature: string | undefined;
  let first: Record<string, unknown> | undefined;
  for (const entry of evidence) {
    validateAllowedKeys('provenance.evidence', entry, EVIDENCE_KEYS);
    const kind = requiredString(entry, 'kind');
    const source = requiredString(entry, 'source');
    const ref = requiredString(entry, 'ref');
    validateRfc3339(
      'evidence.observed_at',
      requiredString(entry, 'observed_at'),
    );
    if (Object.hasOwn(entry, 'digest') && typeof entry.digest !== 'string') {
      invalidModel('evidence.digest must be a string');
    }

    const currentSignature = `${kind}|${source}|${ref}`;
    if (signature === undefined) {
      signature = currentSignature;
      first = entry;
      continue;
    }
    if (signature !== currentSignature) {
      rejected('conflicting deploy authority evidence');
    }
  }

  if (!first) rejected('deploy authority requires evidence');
  const kind = requiredString(first, 'kind');
  const source = requiredString(first, 'source');
  if (kind === 'operator_command' && source === 'release-control-plane') {
    return 'operator_command_authority';
  }
  if (kind === 'factory_batch_manifest' && source === 'partner-factory') {
    return 'unique_factory_manifest_match';
  }
  if (kind === 'codepipeline_execution' && source === 'service-ci') {
    return 'codepipeline_execution_authority';
  }
  if (kind === 'submodule_pin' && source === 'service-ci') {
    return 'unique_submodule_pin_match';
  }
  rejected(`unsupported deploy authority evidence: ${source}/${kind}`);
}

function validateConfidence(
  confidence: Record<string, unknown>,
  expectedReason: string,
): void {
  const level = requiredString(confidence, 'level');
  if (level !== 'high') {
    rejected(`${level} confidence cannot authorize deploy state`);
  }
  const reasons = stringArrayValue(confidence.reasons);
  if (reasons.length !== 1 || reasons[0] !== expectedReason) {
    rejected('confidence reasons do not match deterministic authority');
  }
}

function validateAllowedKeys(
  label: string,
  record: Record<string, unknown>,
  allowed: ReadonlySet<string>,
): void {
  for (const key of Object.keys(record)) {
    if (!allowed.has(key)) {
      invalidModel(`unsupported ${label} key: ${key}`);
    }
  }
}

function requiredString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  if (typeof value !== 'string' || value.length === 0) {
    invalidModel(`${key} must be a non-empty string`);
  }
  return value;
}

function validateRfc3339(label: string, value: string): void {
  if (!RFC3339_RE.test(value) || Number.isNaN(Date.parse(value))) {
    invalidModel(`${label} must be RFC3339`);
  }
}

function recordValue(value: unknown, label: string): Record<string, unknown> {
  if (!isRecord(value)) invalidModel(`${label} must be an object`);
  return value;
}

function evidenceValues(value: unknown): Array<Record<string, unknown>> {
  if (!Array.isArray(value)) {
    invalidModel('provenance.evidence must be an array');
  }
  return value.map((entry) => recordValue(entry, 'provenance.evidence entry'));
}

function stringArrayValue(value: unknown): string[] {
  if (!Array.isArray(value)) {
    invalidModel('confidence.reasons must be a string array');
  }
  return value.map((entry) => {
    if (typeof entry !== 'string' || entry.length === 0) {
      invalidModel('confidence.reasons must contain strings');
    }
    return entry;
  });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function invalidModel(message: string): never {
  throw new TheorydbError('ErrInvalidModel', message);
}

function rejected(message: string): never {
  throw new TheorydbError('ErrRejectedDeployAuthorityEvidence', message);
}
