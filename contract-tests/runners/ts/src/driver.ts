import {
  DeleteTableCommand,
  GetItemCommand,
  PutItemCommand,
  type AttributeValue,
  type DynamoDBClient,
} from "@aws-sdk/client-dynamodb";
import YAML from "yaml";

import { TheorydbClient } from "../../../../ts/src/client.js";
import { parseDmsDocument, getDmsModel } from "../../../../ts/src/dms.js";
import { encodeEncryptedPayload } from "../../../../ts/src/encryption-avjson.js";
import {
  envelopeToAttributeValue,
  type EncryptionProvider,
} from "../../../../ts/src/encryption.js";
import { TheorydbError } from "../../../../ts/src/errors.js";
import { marshalScalar } from "../../../../ts/src/marshal.js";
import {
  defineModel,
  type AttributeSchema,
  type Model,
  type ModelSchema,
} from "../../../../ts/src/model.js";
import { createTable, describeTable } from "../../../../ts/src/schema.js";
import { autoMigrate } from "../../../../ts/src/schema-migration.js";
import type { TransactAction } from "../../../../ts/src/transaction.js";
import {
  transitionReleaseState,
  validateDeployAuthorityMetadata,
} from "../../../../ts/src/release-state.js";
import { createDeterministicEncryptionProvider } from "../../../../ts/src/testkit/index.js";
import type {
  DmsRequest,
  MigrateRequest,
  ReadCondition,
  ReadRequest,
} from "./types.js";
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
  | "ErrMigrationEncryptionRequired"
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
    eventBinding?: Record<string, string>,
  ): Promise<void>;
  validateProvenance(
    model: string,
    item: Record<string, unknown>,
  ): Promise<void>;
  setStepEncryption(mode?: string): Promise<void>;
  rawPut(table: string, rawItem: Record<string, unknown>): Promise<void>;
  seedEncrypted(
    table: string,
    item: Record<string, unknown>,
    envelopeVersion: number,
    corrupt?: string,
    reset?: boolean,
  ): Promise<void>;
  migrate(req: MigrateRequest, targetTable: string): Promise<void>;
  dmsParse(req: DmsRequest): Promise<void>;
  dmsGet(
    req: DmsRequest,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown>>;
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
  private readonly models: Map<string, Model>;
  private readonly ddb: DynamoDBClient;
  private readonly encryption: EncryptionProvider | undefined;
  private noEncryptionClient: TheorydbClient | undefined;
  private stepEncryption = "";

  constructor(
    ddb: DynamoDBClient,
    models: Model[],
    opts: { exactNumbers?: boolean; encryption?: EncryptionProvider } = {},
  ) {
    this.ddb = ddb;
    this.encryption = opts.encryption;
    this.models = new Map(models.map((model) => [model.name, model]));
    this.client = new TheorydbClient(ddb, {
      ...(opts.encryption ? { encryption: opts.encryption } : {}),
    }).register(...models);
  }

  private activeClient(): TheorydbClient {
    if (this.stepEncryption === "none" && this.noEncryptionClient) {
      return this.noEncryptionClient;
    }
    return this.client;
  }

  async setStepEncryption(mode?: string): Promise<void> {
    switch (mode) {
      case undefined:
      case "":
      case "deterministic":
        this.stepEncryption = "";
        return;
      case "none":
        if (!this.noEncryptionClient) {
          this.noEncryptionClient = new TheorydbClient(this.ddb).register(
            ...this.models.values(),
          );
        }
        this.stepEncryption = "none";
        return;
      default:
        throw new TheorydbError(
          "ErrInvalidModel",
          `unsupported step encryption mode ${mode}`,
        );
    }
  }

  capabilities(): readonly string[] {
    return [
      "crud",
      "omitempty",
      "lifecycle.timestamps",
      "optimistic_lock.version",
      "error.version_conflict",
      "ttl.epoch_seconds",
      "number.precision.exact",
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
      "encryption.migration_rebinding",
      "dms.json_string_mode",
    ];
  }

  async create(
    model: string,
    item: Record<string, unknown>,
    opts: { ifNotExists?: boolean },
  ): Promise<void> {
    validateReleaseStateMetadataIfPresent(model, item);
    await this.activeClient().create(model, contractItemForModel(model, item), {
      ifNotExists: opts.ifNotExists,
    });
  }

  async get(
    model: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return await this.activeClient().get(model, key);
  }

  async getOptional(
    model: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown> | undefined> {
    return (await this.activeClient().getOrNull(model, key)) ?? undefined;
  }

  async update(
    model: string,
    item: Record<string, unknown>,
    fields: string[],
    opts?: { protectedAttributes?: readonly string[] },
  ): Promise<void> {
    await this.activeClient().update(
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
    await this.activeClient().save(model, contractItemForModel(model, item));
  }

  async delete(model: string, key: Record<string, unknown>): Promise<void> {
    await this.activeClient().delete(model, key);
  }

  async query(model: string, req: ReadRequest): Promise<ReadResult> {
    let builder = this.activeClient().query(model);
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
    let builder = this.activeClient().scan(model);
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
    let builder = this.activeClient().query(model);
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
    let builder = this.activeClient().scan(model);
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
    const rows = await this.activeClient().transactGet(
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
    const result = await this.activeClient().batchGet(model, keys);
    return { items: result.items };
  }

  async batchWrite(
    model: string,
    puts: Array<Record<string, unknown>>,
    deletes: Array<Record<string, unknown>>,
  ): Promise<void> {
    const result = await this.activeClient().batchWrite(model, {
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
    await this.activeClient().transactWrite(
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
    return {
      kind: "update",
      model: modelName,
      item: contractItemForModel(modelName, {
        ...action.key,
        ...action.set,
      }),
      fields: Object.keys(action.set),
      conditionExpression: action.conditionExpression,
      expressionAttributeNames: action.expressionAttributeNames,
      expressionAttributeValues: this.expressionValues(
        modelName,
        action.expressionAttributeValues,
      ),
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
    eventBinding?: Record<string, string>,
  ): Promise<void> {
    await transitionReleaseState(this.client, {
      actualModel: actual.model,
      actualKey: actual.key,
      set: actual.set,
      eventModel: event.model,
      eventItem: this.exactNumericTransitionItem(event.model, event.item),
      expectedVersion: actual.expectedVersion,
      eventBinding,
    });
  }

  // Scenario YAML keeps integers above 2^53 as quoted decimal strings so every
  // parser stays exact. Convert only N-typed event attributes to bigint; S
  // attributes stay strings, preserving the DynamoDB type discriminator.
  private exactNumericTransitionItem(
    modelName: string,
    item: Record<string, unknown>,
  ): Record<string, unknown> {
    const model = this.requireScenarioModel(modelName);
    const out = { ...item };
    for (const [attribute, schema] of model.attributes) {
      if (schema.type !== "N") continue;
      const value = out[attribute];
      if (typeof value === "string" && /^-?\d+$/.test(value)) {
        out[attribute] = BigInt(value);
      }
    }
    return out;
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

  async rawPut(
    table: string,
    rawItem: Record<string, unknown>,
  ): Promise<void> {
    const item: Record<string, AttributeValue> = {};
    for (const [name, raw] of Object.entries(rawItem)) {
      item[name] = decodeRawAttributeValue(raw);
    }
    await this.ddb.send(
      new PutItemCommand({ TableName: table, Item: item }),
    );
  }

  async seedEncrypted(
    table: string,
    item: Record<string, unknown>,
    envelopeVersion: number,
    corrupt?: string,
    reset?: boolean,
  ): Promise<void> {
    const sourceModel = this.migrationSourceModel(table);
    if (reset) {
      await this.resetMigrationSourceTable(table);
    }
    await this.ensureMigrationSourceTable(sourceModel);
    const pkName = sourceModel.roles.pk;
    const pkValue = item[pkName];
    const skName = sourceModel.roles.sk;
    const skValue = skName ? item[skName] : undefined;

    if (envelopeVersion === 2) {
      const client = new TheorydbClient(this.ddb, {
        ...(this.encryption ? { encryption: this.encryption } : {}),
      }).register(sourceModel);
      await client.save(sourceModel.name, item);
    } else {
      if (!this.encryption) {
        throw new TheorydbError(
          "ErrMigrationEncryptionRequired",
          "seed_encrypted v1 requires the deterministic encryption provider",
        );
      }
      const env = await this.encryption.encrypt(
        encodeEncryptedPayload({ S: String(item.secret ?? "") }),
        { model: sourceModel.name, attribute: "secret" },
      );
      const stored: Record<string, AttributeValue> = {
        PK: marshalScalar(requireAttribute(sourceModel, "PK"), pkValue),
        secret: envelopeToAttributeValue(env),
      };
      if (skName) {
        stored[skName] = marshalScalar(
          requireAttribute(sourceModel, skName),
          skValue,
        );
      }
      await this.ddb.send(
        new PutItemCommand({ TableName: table, Item: stored }),
      );
    }

    if (corrupt) {
      await this.corruptStoredEnvelope(
        table,
        sourceModel,
        pkValue,
        skValue,
        corrupt,
      );
    }
  }

  async migrate(req: MigrateRequest, targetTable: string): Promise<void> {
    const target = this.modelByTable(targetTable);
    const sourceModel = this.migrationSourceModel(req.source_table);
    let encryption: EncryptionProvider | undefined;
    if (req.encryption === "failing") {
      encryption = failingMigrationProvider;
    } else if (req.encryption !== "none") {
      encryption = this.encryption;
    }
    await autoMigrate(this.ddb, sourceModel, {
      targetModel: target,
      dataCopy: true,
      batchSize: req.batch_size ?? 25,
      ...(encryption ? { encryption } : {}),
    });
  }

  async dmsParse(req: DmsRequest): Promise<void> {
    if (req.document !== undefined) {
      parseDmsDocument(req.document);
      return;
    }
    defineModel(parseDmsSchema(req.schema));
  }

  async dmsGet(
    req: DmsRequest,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    const schema =
      req.document !== undefined
        ? getDmsModel(parseDmsDocument(req.document), req.model)
        : parseDmsSchema(req.schema);
    const model = defineModel(schema);
    const client = new TheorydbClient(this.ddb).register(model);
    return await client.get(req.model, key);
  }

  private modelByTable(tableName: string): Model {
    for (const model of this.models.values()) {
      if (model.tableName === tableName) return model;
    }
    throw new TheorydbError(
      "ErrInvalidModel",
      `unknown model for table: ${tableName}`,
    );
  }

  private migrationSourceModel(tableName: string): Model {
    const target = this.requireScenarioModel("EncryptedRecord");
    const schema = {
      ...target.schema,
      name: "MigrationSource",
      table: { name: tableName },
    } as ModelSchema;
    return defineModel(schema);
  }

  private async resetMigrationSourceTable(table: string): Promise<void> {
    try {
      await this.ddb.send(
        new DeleteTableCommand({ TableName: table }),
      );
    } catch (err) {
      const name = (err as { name?: string }).name;
      if (name !== "ResourceNotFoundException") throw err;
    }
  }

  private async ensureMigrationSourceTable(model: Model): Promise<void> {
    try {
      await describeTable(this.ddb, model);
    } catch {
      await createTable(this.ddb, model);
    }
  }

  private async corruptStoredEnvelope(
    table: string,
    model: Model,
    pkValue: unknown,
    skValue: unknown,
    mode: string,
  ): Promise<void> {
    const pkName = model.roles.pk;
    const key: Record<string, AttributeValue> = {
      [pkName]: marshalScalar(requireAttribute(model, pkName), pkValue),
    };
    if (model.roles.sk) {
      key[model.roles.sk] = marshalScalar(
        requireAttribute(model, model.roles.sk),
        skValue,
      );
    }
    const out = await this.ddb.send(
      new GetItemCommand({
        TableName: table,
        Key: key,
        ConsistentRead: true,
      }),
    );
    const envelope = out.Item?.secret;
    if (!envelope || !("M" in envelope) || !envelope.M) {
      throw new TheorydbError(
        "ErrInvalidEncryptedEnvelope",
        "stored migration row has no encrypted envelope",
      );
    }
    const map = { ...envelope.M };
    if (mode === "nonce") {
      map.nonce = { B: Buffer.from([1, 2, 3, 4, 5, 6, 7, 8]) };
    } else if (mode === "ciphertext") {
      const ct = map.ct;
      if (!ct || !("B" in ct) || !ct.B || ct.B.length === 0) {
        throw new TheorydbError(
          "ErrInvalidEncryptedEnvelope",
          "stored envelope has no ciphertext",
        );
      }
      const tampered = new Uint8Array(ct.B);
      tampered[tampered.length - 1] ^= 0xff;
      map.ct = { B: tampered };
    } else {
      throw new TheorydbError(
        "ErrInvalidModel",
        `unsupported corrupt mode: ${mode}`,
      );
    }
    await this.ddb.send(
      new PutItemCommand({
        TableName: table,
        Item: { ...out.Item, secret: { M: map } },
      }),
    );
  }
}

const failingMigrationProvider: EncryptionProvider = {
  async encrypt(): Promise<never> {
    throw new TheorydbError(
      "ErrMigrationEncryptionRequired",
      "contract failing encryption provider",
    );
  },
  async decrypt(): Promise<never> {
    throw new TheorydbError(
      "ErrMigrationEncryptionRequired",
      "contract failing encryption provider",
    );
  },
};

function requireAttribute(model: Model, name: string): AttributeSchema {
  const attr = model.attributes.get(name);
  if (!attr) {
    throw new TheorydbError(
      "ErrInvalidModel",
      `unknown attribute for model ${model.name}: ${name}`,
    );
  }
  return attr;
}

function parseDmsSchema(raw: string | undefined): ModelSchema {
  if (raw === undefined) {
    throw new TheorydbError("ErrInvalidModel", "DMS schema is required");
  }
  return YAML.parse(raw) as ModelSchema;
}

function decodeRawAttributeValue(raw: unknown): AttributeValue {
  if (typeof raw !== "object" || raw === null) {
    throw new TheorydbError("ErrInvalidModel", "raw value must be an object");
  }
  const values = raw as Record<string, unknown>;
  if ("s" in values) return { S: String(values.s) };
  if ("n" in values) return { N: String(values.n) };
  if ("b64" in values) {
    return { B: Buffer.from(String(values.b64), "base64") };
  }
  if ("bool" in values) return { BOOL: values.bool === true };
  if ("null" in values && values.null === true) return { NULL: true };
  if ("envelope" in values) {
    const env = values.envelope as Record<string, unknown>;
    return {
      M: {
        v: { N: String(env.version) },
        edk: { B: Buffer.from(String(env.edk_b64 ?? ""), "base64") },
        nonce: { B: Buffer.from(String(env.nonce_b64 ?? ""), "base64") },
        ct: { B: Buffer.from(String(env.ct_b64 ?? ""), "base64") },
      },
    };
  }
  throw new TheorydbError("ErrInvalidModel", "unsupported raw value shape");
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
