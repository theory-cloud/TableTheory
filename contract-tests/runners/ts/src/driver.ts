import type { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { TheorydbClient } from "../../../../ts/src/client.js";
import type { Model } from "../../../../ts/src/model.js";

export type ErrorCode =
  | "ErrItemNotFound"
  | "ErrConditionFailed"
  | "ErrInvalidModel"
  | "ErrMissingPrimaryKey"
  | "ErrInvalidOperator"
  | "ErrEncryptedFieldNotQueryable"
  | "ErrEncryptionNotConfigured"
  | "ErrInvalidEncryptedEnvelope";

export interface Driver {
  create(model: string, item: Record<string, unknown>, opts: { ifNotExists?: boolean }): Promise<void>;
  get(model: string, key: Record<string, unknown>): Promise<Record<string, unknown>>;
  update(model: string, item: Record<string, unknown>, fields: string[]): Promise<void>;
  delete(model: string, key: Record<string, unknown>): Promise<void>;
}

export class TheorydbDriver implements Driver {
  private readonly client: TheorydbClient;

  constructor(ddb: DynamoDBClient, models: Model[]) {
    this.client = new TheorydbClient(ddb).register(...models);
  }

  async create(model: string, item: Record<string, unknown>, opts: { ifNotExists?: boolean }): Promise<void> {
    await this.client.create(model, item, { ifNotExists: opts.ifNotExists });
  }

  async get(model: string, key: Record<string, unknown>): Promise<Record<string, unknown>> {
    return await this.client.get(model, key);
  }

  async update(model: string, item: Record<string, unknown>, fields: string[]): Promise<void> {
    await this.client.update(model, item, fields);
  }

  async delete(model: string, key: Record<string, unknown>): Promise<void> {
    await this.client.delete(model, key);
  }
}
