import type { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { TheorydbClient } from "../../../../ts/src/client.js";
import { TheorydbError } from "../../../../ts/src/errors.js";
import type { Model } from "../../../../ts/src/model.js";
import {
  transitionReleaseState,
  validateDeployAuthorityMetadata,
} from "../../../../ts/src/release-state.js";

export type ErrorCode =
  | "ErrItemNotFound"
  | "ErrConditionFailed"
  | "ErrInvalidModel"
  | "ErrMissingPrimaryKey"
  | "ErrInvalidOperator"
  | "ErrEncryptedFieldNotQueryable"
  | "ErrEncryptionNotConfigured"
  | "ErrInvalidEncryptedEnvelope"
  | "ErrImmutableModelMutation"
  | "ErrProtectedFieldMutation"
  | "ErrRejectedDeployAuthorityEvidence";

export interface Driver {
  capabilities(): readonly string[];
  create(
    model: string,
    item: Record<string, unknown>,
    opts: { ifNotExists?: boolean },
  ): Promise<void>;
  get(
    model: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown>>;
  update(
    model: string,
    item: Record<string, unknown>,
    fields: string[],
    opts?: { protectedAttributes?: readonly string[] },
  ): Promise<void>;
  save(model: string, item: Record<string, unknown>): Promise<void>;
  delete(model: string, key: Record<string, unknown>): Promise<void>;
  transitionAppendEvent(
    actual: TransitionActual,
    event: TransitionEvent,
  ): Promise<void>;
  validateProvenance(
    model: string,
    item: Record<string, unknown>,
  ): Promise<void>;
}

export interface TransitionActual {
  model: string;
  key: Record<string, unknown>;
  set: Record<string, unknown>;
  expectedVersion?: number;
}

export interface TransitionEvent {
  model: string;
  item: Record<string, unknown>;
}

export class TheorydbDriver implements Driver {
  private readonly client: TheorydbClient;

  constructor(ddb: DynamoDBClient, models: Model[]) {
    this.client = new TheorydbClient(ddb).register(...models);
  }

  capabilities(): readonly string[] {
    return [
      "crud",
      "omitempty",
      "lifecycle.timestamps",
      "optimistic_lock.version",
      "ttl.epoch_seconds",
      "release_state.write_policy",
      "release_state.transactional_transition",
      "release_state.provenance_confidence",
    ];
  }

  async create(
    model: string,
    item: Record<string, unknown>,
    opts: { ifNotExists?: boolean },
  ): Promise<void> {
    validateReleaseStateMetadataIfPresent(model, item);
    await this.client.create(model, item, { ifNotExists: opts.ifNotExists });
  }

  async get(
    model: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return await this.client.get(model, key);
  }

  async update(
    model: string,
    item: Record<string, unknown>,
    fields: string[],
    opts?: { protectedAttributes?: readonly string[] },
  ): Promise<void> {
    await this.client.update(
      model,
      item,
      fields,
      opts?.protectedAttributes
        ? { protectedAttributes: opts.protectedAttributes }
        : {},
    );
  }

  async save(model: string, item: Record<string, unknown>): Promise<void> {
    validateReleaseStateMetadataIfPresent(model, item);
    await this.client.save(model, item);
  }

  async delete(model: string, key: Record<string, unknown>): Promise<void> {
    await this.client.delete(model, key);
  }

  async transitionAppendEvent(
    actual: TransitionActual,
    event: TransitionEvent,
  ): Promise<void> {
    await transitionReleaseState(this.client, {
      actualModel: actual.model,
      actualKey: actual.key,
      set: actual.set,
      eventModel: event.model,
      eventItem: event.item,
      expectedVersion: actual.expectedVersion,
    });
  }

  async validateProvenance(
    model: string,
    item: Record<string, unknown>,
  ): Promise<void> {
    if (model !== "ReleaseStateActual") {
      throw new TheorydbError(
        "ErrInvalidModel",
        `validate_provenance unsupported for ${model}`,
      );
    }
    validateDeployAuthorityMetadata(item);
  }
}

function validateReleaseStateMetadataIfPresent(
  model: string,
  item: Record<string, unknown>,
): void {
  if (model !== "ReleaseStateActual") return;
  if (Object.hasOwn(item, "provenance") || Object.hasOwn(item, "confidence")) {
    validateDeployAuthorityMetadata(item);
  }
}
