import type { AttributeValue, DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { TheorydbClient } from "../../../../ts/src/client.js";
import type { EncryptionProvider } from "../../../../ts/src/encryption.js";
import { TheorydbError } from "../../../../ts/src/errors.js";
import { marshalScalar } from "../../../../ts/src/marshal.js";
import type { Model } from "../../../../ts/src/model.js";
import type { TransactAction } from "../../../../ts/src/transaction.js";
import {
  transitionReleaseState,
  validateDeployAuthorityMetadata,
} from "../../../../ts/src/release-state.js";
import { createDeterministicEncryptionProvider } from "../../../../ts/src/testkit/index.js";
import type { ReadCondition, ReadRequest } from "./types.js";
import type { EncryptionScenarioConfig } from "./types.js";

export type ErrorCode =
  | "ErrItemNotFound"
  | "ErrConditionFailed"
  | "ErrVersionConflict"
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
  getOptional(
    model: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown> | undefined>;
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
  countQuery(model: string, req: ReadRequest): Promise<ReadResult>;
  countScan(model: string, req: ReadRequest): Promise<ReadResult>;
  transactGet(model: string, items: KeyedItem[]): Promise<ReadResult>;
  batchGet(
    model: string,
    keys: Array<Record<string, unknown>>,
  ): Promise<ReadResult>;
  batchWrite(
    model: string,
    puts: Array<Record<string, unknown>>,
    deletes: Array<Record<string, unknown>>,
  ): Promise<void>;
  transactWrite(
    model: string,
    actions: ScenarioTransactWriteAction[],
  ): Promise<void>;
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
  count?: number;
}

export interface KeyedItem {
  model?: string;
  key: Record<string, unknown>;
}

export interface ScenarioTransactWriteAction {
  kind: string;
  model?: string;
  item?: Record<string, unknown>;
  key?: Record<string, unknown>;
  set?: Record<string, unknown>;
  conditionExpression?: string;
  expressionAttributeNames?: Record<string, string>;
  expressionAttributeValues?: Record<string, unknown>;
  ifNotExists?: boolean;
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
  private readonly models: Map<string, Model>;

  constructor(
    ddb: DynamoDBClient,
    models: Model[],
    opts: { exactNumbers?: boolean; encryption?: EncryptionProvider } = {},
  ) {
    this.exactNumbers = opts.exactNumbers ?? false;
    this.models = new Map(models.map((model) => [model.name, model]));
    this.client = new TheorydbClient(ddb, {
      ...(this.exactNumbers ? { numberUnmarshalMode: "string" } : {}),
      ...(opts.encryption ? { encryption: opts.encryption } : {}),
    }).register(...models);
  }

  capabilities(): readonly string[] {
    return [
      "crud",
      "omitempty",
      "lifecycle.timestamps",
      "optimistic_lock.version",
      "error.version_conflict",
      "ttl.epoch_seconds",
      ...(this.exactNumbers ? ["number.precision.exact"] : []),
      "type.matrix",
      "query.basic",
      "scan.basic",
      "count.native",
      "get.optional",
      "transact_get",
      "batch.get",
      "batch.write",
      "transact.write",
      "release_state.write_policy",
      "release_state.transactional_transition",
      "release_state.provenance_confidence",
      "encryption.fail_closed",
      "encryption.deterministic_interop",
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

  async getOptional(
    model: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown> | undefined> {
    return (await this.client.getOrNull(model, key)) ?? undefined;
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

  async countQuery(model: string, req: ReadRequest): Promise<ReadResult> {
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
    return { items: [], count: await builder.count() };
  }

  async countScan(model: string, req: ReadRequest): Promise<ReadResult> {
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
    return { items: [], count: await builder.count() };
  }

  async transactGet(model: string, items: KeyedItem[]): Promise<ReadResult> {
    const rows = await this.client.transactGet(
      items.map((item) => ({
        model: item.model ?? model,
        key: item.key,
      })),
    );
    return {
      items: rows.filter(
        (row): row is Record<string, unknown> => row !== undefined,
      ),
    };
  }

  async batchGet(
    model: string,
    keys: Array<Record<string, unknown>>,
  ): Promise<ReadResult> {
    const result = await this.client.batchGet(model, keys);
    return { items: result.items };
  }

  async batchWrite(
    model: string,
    puts: Array<Record<string, unknown>>,
    deletes: Array<Record<string, unknown>>,
  ): Promise<void> {
    const result = await this.client.batchWrite(model, {
      puts: puts.map((item) => contractItemForModel(model, item)),
      deletes,
    });
    if (result.unprocessed.length > 0) {
      throw new TheorydbError(
        "ErrInvalidOperator",
        `batch write left ${result.unprocessed.length} unprocessed items`,
      );
    }
  }

  async transactWrite(
    model: string,
    actions: ScenarioTransactWriteAction[],
  ): Promise<void> {
    await this.client.transactWrite(
      actions.map((action) => this.toTransactAction(model, action)),
    );
  }

  private toTransactAction(
    defaultModel: string,
    action: ScenarioTransactWriteAction,
  ): TransactAction {
    const modelName = action.model ?? defaultModel;
    switch (action.kind.toLowerCase()) {
      case "put":
      case "create":
        if (!action.item) {
          throw new TheorydbError(
            "ErrInvalidModel",
            "put action requires item",
          );
        }
        return {
          kind: "put",
          model: modelName,
          item: contractItemForModel(modelName, action.item),
          ifNotExists:
            action.ifNotExists || action.kind.toLowerCase() === "create",
        };
      case "update":
        return this.toUpdateAction(modelName, action);
      case "delete":
        if (!action.key) {
          throw new TheorydbError(
            "ErrInvalidModel",
            "delete action requires key",
          );
        }
        return {
          kind: "delete",
          model: modelName,
          key: action.key,
          conditionExpression: action.conditionExpression,
          expressionAttributeNames: action.expressionAttributeNames,
          expressionAttributeValues: this.expressionValues(
            modelName,
            action.expressionAttributeValues,
          ),
        };
      case "condition":
      case "condition_check":
        if (!action.key || !action.conditionExpression) {
          throw new TheorydbError(
            "ErrInvalidModel",
            "condition action requires key and conditionExpression",
          );
        }
        return {
          kind: "condition",
          model: modelName,
          key: action.key,
          conditionExpression: action.conditionExpression,
          expressionAttributeNames: action.expressionAttributeNames,
          expressionAttributeValues: this.expressionValues(
            modelName,
            action.expressionAttributeValues,
          ),
        };
      default:
        throw new TheorydbError(
          "ErrInvalidOperator",
          `unsupported transact_write action: ${action.kind}`,
        );
    }
  }

  private toUpdateAction(
    modelName: string,
    action: ScenarioTransactWriteAction,
  ): TransactAction {
    if (!action.key || !action.set) {
      throw new TheorydbError(
        "ErrInvalidModel",
        "update action requires key and set",
      );
    }
    const model = this.requireScenarioModel(modelName);
    const names: Record<string, string> = {
      ...(action.expressionAttributeNames ?? {}),
    };
    const values: Record<string, AttributeValue> =
      this.expressionValues(modelName, action.expressionAttributeValues) ?? {};
    const assignments: string[] = [];
    for (const [index, [field, value]] of Object.entries(
      action.set,
    ).entries()) {
      const attr = model.attributes.get(field);
      if (!attr) {
        throw new TheorydbError(
          "ErrInvalidModel",
          `unknown update field ${field}`,
        );
      }
      const name = `#u${index}`;
      const valueName = `:u${index}`;
      names[name] = attr.attribute;
      values[valueName] = marshalScalar(attr, value);
      assignments.push(`${name} = ${valueName}`);
    }
    return {
      kind: "update",
      model: modelName,
      key: action.key,
      updateExpression: `SET ${assignments.join(", ")}`,
      conditionExpression: action.conditionExpression,
      expressionAttributeNames: names,
      expressionAttributeValues: values,
    };
  }

  private expressionValues(
    modelName: string,
    values: Record<string, unknown> | undefined,
  ): Record<string, AttributeValue> | undefined {
    if (!values) return undefined;
    const model = this.requireScenarioModel(modelName);
    const fallbackSchema =
      model.attributes.get(model.roles.pk) ?? model.schema.attributes[0];
    if (!fallbackSchema) return undefined;
    const out: Record<string, AttributeValue> = {};
    for (const [key, value] of Object.entries(values)) {
      out[key] = marshalScalar(fallbackSchema, value);
    }
    return out;
  }

  private requireScenarioModel(name: string): Model {
    const model = this.models.get(name);
    if (!model) {
      throw new TheorydbError("ErrInvalidModel", `unknown model: ${name}`);
    }
    return model;
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

export function encryptionProviderForScenario(
  config: EncryptionScenarioConfig | undefined,
): EncryptionProvider | undefined {
  if (!config) return undefined;
  if (config.provider !== "deterministic") {
    throw new TheorydbError(
      "ErrInvalidModel",
      `unsupported scenario encryption provider: ${String(config.provider ?? "")}`,
    );
  }
  if (!config.seed) {
    throw new TheorydbError(
      "ErrInvalidModel",
      "deterministic scenario encryption requires seed",
    );
  }
  return createDeterministicEncryptionProvider(config.seed);
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
