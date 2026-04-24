import type { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { TheorydbClient } from "../../../../ts/src/client.js";
import { TheorydbError } from "../../../../ts/src/errors.js";
import type { Model } from "../../../../ts/src/model.js";

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
    ];
  }

  async create(
    model: string,
    item: Record<string, unknown>,
    opts: { ifNotExists?: boolean },
  ): Promise<void> {
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
    _opts?: { protectedAttributes?: readonly string[] },
  ): Promise<void> {
    await this.client.update(model, item, fields);
  }

  async save(model: string, _item: Record<string, unknown>): Promise<void> {
    throw new TheorydbError(
      "ErrInvalidModel",
      `save not implemented for contract model ${model}`,
    );
  }

  async delete(model: string, key: Record<string, unknown>): Promise<void> {
    await this.client.delete(model, key);
  }

  async transitionAppendEvent(
    actual: TransitionActual,
    event: TransitionEvent,
  ): Promise<void> {
    throw new TheorydbError(
      "ErrInvalidModel",
      `transition_append_event not implemented for ${actual.model}/${event.model}`,
    );
  }

  async validateProvenance(
    model: string,
    _item: Record<string, unknown>,
  ): Promise<void> {
    throw new TheorydbError(
      "ErrInvalidModel",
      `validate_provenance not implemented for ${model}`,
    );
  }
}
