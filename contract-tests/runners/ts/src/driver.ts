import type { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { TheorydbClient } from "../../../../ts/src/client.js";
import { TheorydbError } from "../../../../ts/src/errors.js";
import type { Model } from "../../../../ts/src/model.js";
import {
  transitionReleaseState,
  validateDeployAuthorityMetadata,
} from "../../../../ts/src/release-state.js";
import type { ReadCondition, ReadRequest } from "./types.js";

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
  query(model: string, req: ReadRequest): Promise<ReadResult>;
  scan(model: string, req: ReadRequest): Promise<ReadResult>;
  transitionAppendEvent(
    actual: TransitionActual,
    event: TransitionEvent,
  ): Promise<void>;
  validateProvenance(
    model: string,
    item: Record<string, unknown>,
  ): Promise<void>;
}

export interface ReadResult {
  items: Array<Record<string, unknown>>;
  cursor?: string;
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
  private readonly exactNumbers: boolean;

  constructor(
    ddb: DynamoDBClient,
    models: Model[],
    opts: { exactNumbers?: boolean } = {},
  ) {
    this.exactNumbers = opts.exactNumbers ?? false;
    this.client = new TheorydbClient(
      ddb,
      this.exactNumbers ? { numberUnmarshalMode: "string" } : {},
    ).register(...models);
  }

  capabilities(): readonly string[] {
    return [
      "crud",
      "omitempty",
      "lifecycle.timestamps",
      "optimistic_lock.version",
      "ttl.epoch_seconds",
      ...(this.exactNumbers ? ["number.precision.exact"] : []),
      "type.matrix",
      "query.basic",
      "scan.basic",
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
    await this.client.create(model, contractItemForModel(model, item), {
      ifNotExists: opts.ifNotExists,
    });
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
      contractItemForModel(model, item),
      fields,
      opts?.protectedAttributes
        ? { protectedAttributes: opts.protectedAttributes }
        : {},
    );
  }

  async save(model: string, item: Record<string, unknown>): Promise<void> {
    validateReleaseStateMetadataIfPresent(model, item);
    await this.client.save(model, contractItemForModel(model, item));
  }

  async delete(model: string, key: Record<string, unknown>): Promise<void> {
    await this.client.delete(model, key);
  }

  async query(model: string, req: ReadRequest): Promise<ReadResult> {
    let builder = this.client.query(model);
    if (req.index) builder = builder.usingIndex(req.index);
    if (!req.partition) {
      throw new TheorydbError(
        "ErrMissingPrimaryKey",
        "query partition is required",
      );
    }
    builder = builder.partitionKey(conditionValue(req.partition));
    if (req.sort) {
      builder = builder.sortKey(
        normalizeSortOperator(req.sort.operator),
        ...conditionValues(req.sort),
      );
    }
    builder = applyReadOptions(builder, req);
    for (const filter of req.filter ?? []) {
      builder = builder.filter(
        filter.attribute,
        filter.operator,
        ...conditionValues(filter),
      );
    }
    const page = await builder.page();
    return { items: page.items, cursor: page.cursor };
  }

  async scan(model: string, req: ReadRequest): Promise<ReadResult> {
    let builder = this.client.scan(model);
    if (req.index) builder = builder.usingIndex(req.index);
    builder = applyReadOptions(builder, req);
    for (const filter of req.filter ?? []) {
      builder = builder.filter(
        filter.attribute,
        filter.operator,
        ...conditionValues(filter),
      );
    }
    const page = await builder.page();
    return { items: page.items, cursor: page.cursor };
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

function contractItemForModel(
  model: string,
  item: Record<string, unknown>,
): Record<string, unknown> {
  if (model !== "TypeMatrix") return item;

  const out: Record<string, unknown> = { ...item };
  if (typeof out.binaryBlob === "string") {
    out.binaryBlob = Buffer.from(out.binaryBlob, "base64");
  }
  if (Array.isArray(out.binarySet)) {
    out.binarySet = out.binarySet.map((value) =>
      typeof value === "string" ? Buffer.from(value, "base64") : value,
    );
  }
  if (Array.isArray(out.emptyBinarySet)) {
    out.emptyBinarySet = out.emptyBinarySet.map((value) =>
      typeof value === "string" ? Buffer.from(value, "base64") : value,
    );
  }
  return out;
}

function applyReadOptions<
  T extends {
    consistentRead(enabled?: boolean): T;
    limit(n: number): T;
    projection(fields: string[]): T;
    cursor(encoded: string): T;
  },
>(builder: T, req: ReadRequest): T {
  let out = builder;
  if (req.consistent_read) out = out.consistentRead(true);
  if (req.limit !== undefined) out = out.limit(req.limit);
  if (req.projection?.length) out = out.projection(req.projection);
  if (req.cursor) out = out.cursor(req.cursor);
  if ("sort" in out && req.sort_direction) {
    out = (out as T & { sort(direction: "ASC" | "DESC"): T }).sort(
      normalizeSortDirection(req.sort_direction),
    );
  }
  return out;
}

function conditionValue(condition: ReadCondition): unknown {
  return condition.values !== undefined ? condition.values : condition.value;
}

function conditionValues(condition: ReadCondition): unknown[] {
  if (condition.values !== undefined) return condition.values;
  if (condition.value === undefined) return [];
  return [condition.value];
}

function normalizeSortOperator(
  op: string,
): "=" | "<" | "<=" | ">" | ">=" | "between" | "begins_with" {
  const normalized = op.toLowerCase();
  switch (normalized) {
    case "=":
    case "<":
    case "<=":
    case ">":
    case ">=":
    case "between":
    case "begins_with":
      return normalized;
    default:
      throw new TheorydbError(
        "ErrInvalidOperator",
        `unsupported sort operator: ${op}`,
      );
  }
}

function normalizeSortDirection(value: string): "ASC" | "DESC" {
  return value.toUpperCase() === "DESC" ? "DESC" : "ASC";
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
