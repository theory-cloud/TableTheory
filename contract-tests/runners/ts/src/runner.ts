import assert from "node:assert/strict";

import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  GetItemCommand,
  ListTablesCommand,
  type AttributeDefinition,
  type AttributeValue,
  type CreateTableCommandInput,
  type DynamoDBClient,
  type GlobalSecondaryIndex,
  type KeySchemaElement,
  type LocalSecondaryIndex,
  type Projection,
} from "@aws-sdk/client-dynamodb";

import { isTheorydbError } from "../../../../ts/src/errors.js";

import type { Driver, ErrorCode } from "./driver.js";
import type { DmsModel, Scenario, Step } from "./types.js";

export async function pingDynamo(ddb: DynamoDBClient): Promise<void> {
  await ddb.send(new ListTablesCommand({ Limit: 1 }));
}

export async function runScenario(opts: {
  ddb: DynamoDBClient;
  driver: Driver;
  scenario: Scenario;
  model: DmsModel;
}): Promise<void> {
  const { ddb, driver, scenario, model } = opts;

  const tableName = scenario.table?.name ?? model.table.name;
  assert.ok(tableName, "table name required");

  await recreateTable(ddb, tableName, model);

  const vars = new Map<string, unknown>();

  for (const step of scenario.steps) {
    await runStep({ ddb, driver, step, scenario, model, tableName, vars });
  }
}

async function runStep(opts: {
  ddb: DynamoDBClient;
  driver: Driver;
  step: Step;
  scenario: Scenario;
  model: DmsModel;
  tableName: string;
  vars: Map<string, unknown>;
}): Promise<void> {
  const { ddb, driver, step, scenario, model, tableName, vars } = opts;

  if (step.op === "sleep") {
    const ms = step.ms ?? 0;
    if (ms > 0) await new Promise((r) => setTimeout(r, ms));
    return;
  }

  if (step.op === "create") {
    const item = step.item ?? {};
    const err = await captureError(() =>
      driver.create(scenario.model, item, { ifNotExists: step.if_not_exists }),
    );
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "update") {
    const item = step.item ?? {};
    const fields = step.fields ?? [];
    const err = await captureError(() =>
      driver.update(scenario.model, item, fields),
    );
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "delete") {
    const key = step.key ?? {};
    const err = await captureError(() => driver.delete(scenario.model, key));
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "get") {
    const key = step.key ?? {};
    const res = await captureResult(() => driver.get(scenario.model, key));

    let raw: Record<string, AttributeValue> | undefined;
    if (!res.err) {
      raw = await getRawItem(ddb, tableName, model, key);
      if (step.save) {
        for (const [varName, attr] of Object.entries(step.save)) {
          vars.set(varName, res.value?.[attr]);
        }
      }
    }

    assertExpectation(step.expect, { err: res.err, item: res.value, raw, model, vars });
    return;
  }

  throw new Error(`unsupported op: ${step.op}`);
}

function assertExpectation(
  expect: Step["expect"] | undefined,
  ctx: {
    err?: unknown;
    item?: Record<string, unknown>;
    raw?: Record<string, AttributeValue>;
    model: DmsModel;
    vars: Map<string, unknown>;
  },
): void {
  if (!expect) return;
  const { err, item, raw, model, vars } = ctx;

  if (expect.error) {
    assert.ok(err, "expected error");
    assert.equal(mapError(err), expect.error);
    return;
  }

  if (expect.ok !== undefined) {
    if (expect.ok) assert.equal(err, undefined);
    else assert.ok(err, "expected failure");
  }

  if (err) return;

  if (expect.item_contains && item) {
    for (const [attr, want] of Object.entries(expect.item_contains)) {
      const have = item[attr];
      assert.ok(attr in item, `missing attr ${attr}`);
      const attrDef = attributeByName(model, attr);
      assert.ok(attrDef, `unknown attr ${attr}`);
      assertValueMatches(attrDef.type, want, have);
    }
  }

  if (expect.item_has_fields && item) {
    for (const attr of expect.item_has_fields) {
      assert.ok(attr in item, `expected field ${attr}`);
    }
  }

  if (expect.item_missing_fields && raw) {
    for (const attr of expect.item_missing_fields) {
      assert.ok(!(attr in raw), `expected missing raw field ${attr}`);
    }
  }

  if (expect.raw_attribute_types && raw) {
    for (const [attr, wantType] of Object.entries(expect.raw_attribute_types)) {
      assert.ok(attr in raw, `expected raw field ${attr}`);
      assert.equal(attributeValueTypeName(raw[attr]!), wantType);
    }
  }

  if (expect.item_field_equals_var && item) {
    for (const [attr, varName] of Object.entries(expect.item_field_equals_var)) {
      assert.equal(item[attr], vars.get(varName));
    }
  }

  if (expect.item_field_not_equals_var && item) {
    for (const [attr, varName] of Object.entries(expect.item_field_not_equals_var)) {
      assert.notEqual(item[attr], vars.get(varName));
    }
  }
}

function attributeByName(model: DmsModel, name: string): DmsModel["attributes"][number] | undefined {
  return model.attributes.find((a) => a.attribute === name);
}

function assertValueMatches(type: string, want: unknown, have: unknown): void {
  switch (type) {
    case "S":
      assert.equal(String(have), String(want));
      return;
    case "N":
      assert.equal(Number(have), Number(want));
      return;
    case "SS": {
      const wantArr = asStringArray(want);
      const haveArr = asStringArray(have);
      wantArr.sort();
      haveArr.sort();
      assert.deepEqual(haveArr, wantArr);
      return;
    }
    default:
      assert.deepEqual(have, want);
  }
}

function asStringArray(value: unknown): string[] {
  if (value === undefined || value === null) return [];
  if (Array.isArray(value)) return value.map((v) => String(v));
  return [String(value)];
}

function mapError(err: unknown): ErrorCode | "" {
  if (isTheorydbError(err)) return err.code;
  return "";
}

function attributeValueTypeName(av: AttributeValue): string {
  if ("S" in av && av.S !== undefined) return "S";
  if ("N" in av && av.N !== undefined) return "N";
  if ("B" in av && av.B !== undefined) return "B";
  if ("BOOL" in av && av.BOOL !== undefined) return "BOOL";
  if ("NULL" in av && av.NULL) return "NULL";
  if ("SS" in av && av.SS !== undefined) return "SS";
  if ("NS" in av && av.NS !== undefined) return "NS";
  if ("BS" in av && av.BS !== undefined) return "BS";
  if ("L" in av && av.L !== undefined) return "L";
  if ("M" in av && av.M !== undefined) return "M";
  return "UNKNOWN";
}

async function captureError(fn: () => Promise<unknown>): Promise<unknown | undefined> {
  try {
    await fn();
    return undefined;
  } catch (err) {
    return err;
  }
}

async function captureResult<T>(fn: () => Promise<T>): Promise<{ value?: T; err?: unknown }> {
  try {
    return { value: await fn() };
  } catch (err) {
    return { err };
  }
}

async function getRawItem(
  ddb: DynamoDBClient,
  tableName: string,
  model: DmsModel,
  key: Record<string, unknown>,
): Promise<Record<string, AttributeValue>> {
  const keyAv = marshalKey(model, key);
  const out = await ddb.send(
    new GetItemCommand({
      TableName: tableName,
      Key: keyAv,
      ConsistentRead: true,
    }),
  );
  if (!out.Item) throw new Error("raw GetItem returned no Item");
  return out.Item;
}

function marshalKey(model: DmsModel, key: Record<string, unknown>): Record<string, AttributeValue> {
  const out: Record<string, AttributeValue> = {};
  const pk = model.keys.partition.attribute;
  out[pk] = scalarToAv(model.keys.partition.type, key[pk]);
  if (model.keys.sort) {
    const sk = model.keys.sort.attribute;
    out[sk] = scalarToAv(model.keys.sort.type, key[sk]);
  }
  return out;
}

function scalarToAv(type: string, value: unknown): AttributeValue {
  switch (type) {
    case "S":
      return { S: String(value ?? "") };
    case "N":
      return { N: String(value ?? "0") };
    case "B":
      if (value instanceof Uint8Array) return { B: value };
      throw new Error("binary key requires Uint8Array");
    default:
      throw new Error(`unsupported key type: ${type}`);
  }
}

async function recreateTable(ddb: DynamoDBClient, tableName: string, model: DmsModel): Promise<void> {
  try {
    await ddb.send(new DeleteTableCommand({ TableName: tableName }));
  } catch (err) {
    if (!isResourceNotFound(err)) throw err;
  }
  await waitTableNotExists(ddb, tableName);

  const input = createTableInput(tableName, model);
  await ddb.send(new CreateTableCommand(input));
  await waitTableExists(ddb, tableName);
}

function isResourceNotFound(err: unknown): boolean {
  return (
    typeof err === "object" &&
    err !== null &&
    "name" in err &&
    (err as { name?: unknown }).name === "ResourceNotFoundException"
  );
}

async function waitTableExists(ddb: DynamoDBClient, tableName: string): Promise<void> {
  for (let i = 0; i < 60; i++) {
    try {
      const resp = await ddb.send(new DescribeTableCommand({ TableName: tableName }));
      if (resp.Table?.TableStatus === "ACTIVE") return;
    } catch (err) {
      if (!isResourceNotFound(err)) throw err;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`timeout waiting for table exists: ${tableName}`);
}

async function waitTableNotExists(ddb: DynamoDBClient, tableName: string): Promise<void> {
  for (let i = 0; i < 40; i++) {
    try {
      await ddb.send(new DescribeTableCommand({ TableName: tableName }));
    } catch (err) {
      if (isResourceNotFound(err)) return;
      throw err;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
}

function createTableInput(tableName: string, model: DmsModel): CreateTableCommandInput {
  const defs = new Map<string, "S" | "N" | "B">();

  const addDef = (attr: { attribute: string; type: "S" | "N" | "B" }): void => {
    defs.set(attr.attribute, attr.type);
  };

  addDef(model.keys.partition);
  if (model.keys.sort) addDef(model.keys.sort);
  for (const idx of model.indexes ?? []) {
    addDef(idx.partition);
    if (idx.sort) addDef(idx.sort);
  }

  const attributeDefinitions: AttributeDefinition[] = Array.from(defs.entries())
    .map(([AttributeName, AttributeType]) => ({ AttributeName, AttributeType }))
    .sort((a, b) => a.AttributeName!.localeCompare(b.AttributeName!));

  const keySchema: KeySchemaElement[] = [
    { AttributeName: model.keys.partition.attribute, KeyType: "HASH" },
  ];
  if (model.keys.sort) {
    keySchema.push({ AttributeName: model.keys.sort.attribute, KeyType: "RANGE" });
  }

  const gsis: GlobalSecondaryIndex[] = [];
  const lsis: LocalSecondaryIndex[] = [];

  for (const idx of model.indexes ?? []) {
    const projection: Projection = {
      ProjectionType: idx.projection?.type ?? "ALL",
      NonKeyAttributes: idx.projection?.fields,
    };

    const indexKeySchema: KeySchemaElement[] = [
      { AttributeName: idx.partition.attribute, KeyType: "HASH" },
    ];
    if (idx.sort) {
      indexKeySchema.push({ AttributeName: idx.sort.attribute, KeyType: "RANGE" });
    }

    if (idx.type === "LSI") {
      lsis.push({
        IndexName: idx.name,
        KeySchema: indexKeySchema,
        Projection: projection,
      });
      continue;
    }

    gsis.push({
      IndexName: idx.name,
      KeySchema: indexKeySchema,
      Projection: projection,
    });
  }

  return {
    TableName: tableName,
    AttributeDefinitions: attributeDefinitions,
    KeySchema: keySchema,
    BillingMode: "PAY_PER_REQUEST",
    GlobalSecondaryIndexes: gsis.length ? gsis : undefined,
    LocalSecondaryIndexes: lsis.length ? lsis : undefined,
  };
}

