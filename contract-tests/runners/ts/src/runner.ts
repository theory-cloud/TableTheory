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
  models: ReadonlyMap<string, DmsModel>;
}): Promise<void> {
  const { ddb, driver, scenario, models } = opts;
  const model = modelByName(models, scenario.model);

  const tableName = scenario.table?.name ?? model.table.name;
  assert.ok(tableName, "table name required");

  const { steps, recreate } = stepsForRuntime(scenario, "ts");
  if (recreate) {
    await recreateTable(ddb, tableName, model);
  }

  const vars = new Map<string, unknown>();

  for (const step of steps) {
    await runStep({ ddb, driver, step, scenario, models, tableName, vars });
  }
}

function stepsForRuntime(
  scenario: Scenario,
  runtimeName: "go" | "ts" | "py",
): { steps: Step[]; recreate: boolean } {
  if (!scenario.seed_runtime) return { steps: scenario.steps, recreate: true };
  if (scenario.seed_runtime.toLowerCase() === runtimeName) {
    return {
      steps: [...(scenario.seed_steps ?? []), ...(scenario.read_steps ?? [])],
      recreate: true,
    };
  }
  return { steps: scenario.read_steps ?? [], recreate: false };
}

async function runStep(opts: {
  ddb: DynamoDBClient;
  driver: Driver;
  step: Step;
  scenario: Scenario;
  models: ReadonlyMap<string, DmsModel>;
  tableName: string;
  vars: Map<string, unknown>;
}): Promise<void> {
  const { ddb, driver, step, scenario, models, tableName, vars } = opts;

  if (step.op === "sleep") {
    const ms = step.ms ?? 0;
    if (ms > 0) await new Promise((r) => setTimeout(r, ms));
    return;
  }

  const modelName = step.model ?? scenario.model;
  const model = modelByName(models, modelName);

  if (step.op === "create") {
    const item = step.item ?? {};
    const err = await captureError(() =>
      driver.create(modelName, item, { ifNotExists: step.if_not_exists }),
    );
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "update") {
    const item = step.item ?? {};
    const fields = step.fields ?? [];
    const err = await captureError(() =>
      driver.update(modelName, item, fields, {
        protectedAttributes: step.protected_attributes,
      }),
    );
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "save") {
    const item = step.item ?? {};
    const err = await captureError(() => driver.save(modelName, item));
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "delete") {
    const key = step.key ?? {};
    const err = await captureError(() => driver.delete(modelName, key));
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "get") {
    const key = step.key ?? {};
    const res = await captureResult(() => driver.get(modelName, key));

    let raw: Record<string, AttributeValue> | undefined;
    if (!res.err) {
      raw = await getRawItem(ddb, tableName, model, key);
      if (step.save) {
        for (const [varName, attr] of Object.entries(step.save)) {
          vars.set(varName, res.value?.[attr]);
        }
      }
    }

    assertExpectation(step.expect, {
      err: res.err,
      item: res.value,
      raw,
      model,
      vars,
    });
    return;
  }

  if (step.op === "query") {
    assert.ok(step.query, "query request is required");
    const res = await captureResult(() => driver.query(modelName, step.query!));
    assertReadExpectation(step.expect, {
      err: res.err,
      result: res.value,
      model,
    });
    return;
  }

  if (step.op === "scan") {
    assert.ok(step.scan, "scan request is required");
    const res = await captureResult(() => driver.scan(modelName, step.scan!));
    assertReadExpectation(step.expect, {
      err: res.err,
      result: res.value,
      model,
    });
    return;
  }

  if (step.op === "transition_append_event") {
    assert.ok(step.actual, "transition_append_event actual is required");
    assert.ok(step.event, "transition_append_event event is required");
    const { actual, event } = step;
    const err = await captureError(() =>
      driver.transitionAppendEvent(
        {
          model: actual.model,
          key: actual.key,
          set: actual.set,
          expectedVersion: actual.expected_version,
        },
        {
          model: event.model,
          item: event.item,
        },
      ),
    );
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  if (step.op === "validate_provenance") {
    const item = step.item ?? {};
    const err = await captureError(() =>
      driver.validateProvenance(modelName, item),
    );
    assertExpectation(step.expect, { err, model, vars });
    return;
  }

  throw new Error(`unsupported op: ${step.op}`);
}

function assertReadExpectation(
  expect: Step["expect"] | undefined,
  ctx: {
    err?: unknown;
    result?: { items: Array<Record<string, unknown>>; cursor?: string };
    model: DmsModel;
  },
): void {
  if (!expect) return;
  const { err, result, model } = ctx;
  const hasReadAssertion = expectationHasAnyKey(expect, [
    "item_count",
    "items_contains",
    "items_missing_fields",
    "cursor_equals",
  ]);

  if (expect.error) {
    assert.ok(err, "expected error");
    assert.equal(mapError(err), expect.error);
    assert.equal(
      hasReadAssertion,
      false,
      "read assertions cannot be combined with error expectations",
    );
    return;
  }
  if (expect.errors?.length) {
    assert.ok(err, "expected error");
    assert.deepEqual(mapErrors(err).sort(), expect.errors.slice().sort());
    assert.equal(
      hasReadAssertion,
      false,
      "read assertions cannot be combined with error expectations",
    );
    return;
  }

  if (hasReadAssertion) {
    assert.equal(
      err,
      undefined,
      "expected successful read for read assertions",
    );
    assert.ok(result, "expected read result for read assertions");
  }

  if (expect.ok !== undefined) {
    if (expect.ok) assert.equal(err, undefined);
    else assert.ok(err, "expected failure");
  }

  if (err) return;
  assert.ok(result, "expected read result");

  if (expect.item_count !== undefined) {
    assert.equal(result.items.length, expect.item_count);
  }

  if (expect.items_contains) {
    assert.ok(
      result.items.length >= expect.items_contains.length,
      "not enough result items",
    );
    for (const [index, wantItem] of expect.items_contains.entries()) {
      const haveItem = result.items[index]!;
      for (const [attr, want] of Object.entries(wantItem)) {
        assert.ok(attr in haveItem, `missing attr ${attr} in result ${index}`);
        const attrDef = attributeByName(model, attr);
        assert.ok(attrDef, `unknown attr ${attr}`);
        assertValueMatches(attrDef.type, want, haveItem[attr]);
      }
    }
  }

  if (expect.items_missing_fields) {
    for (const [index, item] of result.items.entries()) {
      for (const attr of expect.items_missing_fields) {
        assert.ok(
          semanticallyMissingReadField(item[attr], attr in item),
          `expected missing attr ${attr} in result ${index}`,
        );
      }
    }
  }

  if (expect.cursor_equals !== undefined) {
    assert.equal(result.cursor ?? "", expect.cursor_equals);
  }
}

function semanticallyMissingReadField(
  value: unknown,
  present: boolean,
): boolean {
  if (!present || value === null || value === undefined) return true;
  if (typeof value === "string") return value === "";
  if (typeof value === "number") return value === 0;
  if (typeof value === "bigint") return value === 0n;
  if (typeof value === "boolean") return value === false;
  if (Array.isArray(value)) return value.length === 0;
  if (value instanceof Uint8Array) return value.length === 0;
  if (typeof value === "object") return Object.keys(value).length === 0;
  return false;
}

function modelByName(
  models: ReadonlyMap<string, DmsModel>,
  name: string,
): DmsModel {
  const model = models.get(name);
  assert.ok(model, `unknown model: ${name}`);
  return model;
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
  const hasItemAssertion = expectationHasAnyKey(expect, [
    "item_contains",
    "item_equals",
    "item_has_fields",
    "item_missing_fields",
    "raw_attribute_types",
    "item_field_equals_var",
    "item_field_not_equals_var",
  ]);
  const hasRawAssertion = expectationHasAnyKey(expect, [
    "item_missing_fields",
    "raw_attribute_types",
    "raw_item_contains",
  ]);

  if (expect.error) {
    assert.ok(err, "expected error");
    assert.equal(mapError(err), expect.error);
    assert.equal(
      hasItemAssertion,
      false,
      "item assertions cannot be combined with error expectations",
    );
    return;
  }
  if (expect.errors?.length) {
    assert.ok(err, "expected error");
    assert.deepEqual(mapErrors(err).sort(), expect.errors.slice().sort());
    assert.equal(
      hasItemAssertion,
      false,
      "item assertions cannot be combined with error expectations",
    );
    return;
  }

  if (hasItemAssertion) {
    assert.equal(
      err,
      undefined,
      "expected successful operation for item assertions",
    );
    assert.ok(item, "expected item for item assertions");
  }
  if (hasRawAssertion) {
    assert.ok(raw, "expected raw item for raw assertions");
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
      const rawValue =
        attrDef.encryption !== undefined ? undefined : raw?.[attr];
      assertValueMatches(attrDef.type, want, have, rawValue);
    }
  }

  if (expect.item_equals && item) {
    assertItemEquals(expect.item_equals, item, raw, model);
  }

  if (expect.raw_item_contains && raw) {
    for (const [attr, want] of Object.entries(expect.raw_item_contains)) {
      const rawValue = raw[attr];
      assert.ok(rawValue, `missing raw attr ${attr}`);
      assert.deepEqual(
        attributeValueToComparable(rawValue),
        normalizeDocument(want),
        `raw attr ${attr}`,
      );
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
    for (const [attr, varName] of Object.entries(
      expect.item_field_equals_var,
    )) {
      assert.equal(item[attr], vars.get(varName));
    }
  }

  if (expect.item_field_not_equals_var && item) {
    for (const [attr, varName] of Object.entries(
      expect.item_field_not_equals_var,
    )) {
      assert.notEqual(item[attr], vars.get(varName));
    }
  }
}

function expectationHasAnyKey(
  expect: NonNullable<Step["expect"]>,
  keys: readonly (keyof NonNullable<Step["expect"]>)[],
): boolean {
  return keys.some((key) => Object.hasOwn(expect, key));
}

function assertItemEquals(
  want: Record<string, unknown>,
  item: Record<string, unknown>,
  raw: Record<string, AttributeValue> | undefined,
  model: DmsModel,
): void {
  if (raw) {
    assert.deepEqual(
      Object.keys(raw).sort(),
      Object.keys(want).sort(),
      "raw item should contain exactly the expected attributes",
    );
    for (const [attr, wantValue] of Object.entries(want)) {
      const rawValue = raw[attr];
      assert.ok(rawValue, `missing raw attr ${attr}`);
      const attrDef = attributeByName(model, attr);
      assert.ok(attrDef, `unknown attr ${attr}`);
      const rawForValue =
        attrDef.encryption !== undefined ? undefined : rawValue;
      assertValueMatches(attrDef.type, wantValue, item[attr], rawForValue);
    }
    return;
  }

  assert.deepEqual(
    Object.keys(item).sort(),
    Object.keys(want).sort(),
    "item should contain exactly the expected attributes",
  );
  for (const [attr, wantValue] of Object.entries(want)) {
    const attrDef = attributeByName(model, attr);
    assert.ok(attrDef, `unknown attr ${attr}`);
    assertValueMatches(attrDef.type, wantValue, item[attr]);
  }
}

function attributeByName(
  model: DmsModel,
  name: string,
): DmsModel["attributes"][number] | undefined {
  return model.attributes.find((a) => a.attribute === name);
}

function assertValueMatches(
  type: string,
  want: unknown,
  have: unknown,
  raw?: AttributeValue,
): void {
  switch (type) {
    case "S":
      if (raw) {
        assert.ok("S" in raw && raw.S !== undefined, "expected raw S value");
        assert.equal(raw.S, String(want));
        return;
      }
      assert.equal(String(have), String(want));
      return;
    case "N":
      if (raw) {
        assert.ok("N" in raw && raw.N !== undefined, "expected raw N value");
        assert.equal(raw.N, canonicalDecimalString(want));
        return;
      }
      assert.equal(canonicalDecimalString(have), canonicalDecimalString(want));
      return;
    case "B": {
      const wantB = asBase64String(want);
      const haveB =
        raw && "B" in raw && raw.B !== undefined
          ? Buffer.from(raw.B).toString("base64")
          : asBase64String(have);
      assert.equal(haveB, wantB);
      return;
    }
    case "BOOL":
      assert.equal(raw && "BOOL" in raw ? raw.BOOL : have, want);
      return;
    case "NULL":
      if (raw) {
        assert.ok(
          "NULL" in raw && raw.NULL === true,
          "expected raw NULL value",
        );
        return;
      }
      assert.equal(have, null);
      return;
    case "SS": {
      const wantArr = asStringArray(want);
      const haveArr =
        raw && "SS" in raw && raw.SS !== undefined
          ? raw.SS.slice()
          : asStringArray(have);
      wantArr.sort();
      haveArr.sort();
      assert.deepEqual(haveArr, wantArr);
      return;
    }
    case "NS": {
      const wantArr = asNumberArray(want);
      if (raw && "NULL" in raw && raw.NULL === true) {
        assert.deepEqual(wantArr, []);
        return;
      }
      const haveArr =
        raw && "NS" in raw && raw.NS !== undefined
          ? raw.NS.slice()
          : asNumberArray(have);
      wantArr.sort();
      haveArr.sort();
      assert.deepEqual(haveArr, wantArr);
      return;
    }
    case "BS": {
      const wantArr = asBinaryArray(want);
      if (raw && "NULL" in raw && raw.NULL === true) {
        assert.deepEqual(wantArr, []);
        return;
      }
      const haveArr =
        raw && "BS" in raw && raw.BS !== undefined
          ? raw.BS.map((b) => Buffer.from(b).toString("base64"))
          : asBinaryArray(have);
      wantArr.sort();
      haveArr.sort();
      assert.deepEqual(haveArr, wantArr);
      return;
    }
    case "L":
    case "M": {
      const haveDoc = raw
        ? attributeValueToComparable(raw)
        : normalizeDocument(have);
      assert.deepEqual(haveDoc, normalizeDocument(want));
      return;
    }
    default:
      assert.deepEqual(have, want);
  }
}

function asBase64String(value: unknown): string {
  if (typeof value === "string") return value;
  if (value instanceof Uint8Array) return Buffer.from(value).toString("base64");
  throw new Error(`expected base64 string or bytes, got ${typeof value}`);
}

function asNumberArray(value: unknown): string[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value)) return [canonicalDecimalString(value)];
  return value.map((v) => canonicalDecimalString(v));
}

function asBinaryArray(value: unknown): string[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value)) return [asBase64String(value)];
  return value.map((v) => asBase64String(v));
}

function normalizeDocument(value: unknown): unknown {
  if (value instanceof Uint8Array) return Buffer.from(value).toString("base64");
  if (Array.isArray(value)) return value.map((item) => normalizeDocument(item));
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(value)) {
      out[key] = normalizeDocument(child);
    }
    return out;
  }
  return value;
}

function attributeValueToComparable(av: AttributeValue): unknown {
  if ("S" in av && av.S !== undefined) return av.S;
  if ("N" in av && av.N !== undefined) return av.N;
  if ("B" in av && av.B !== undefined)
    return Buffer.from(av.B).toString("base64");
  if ("BOOL" in av && av.BOOL !== undefined) return av.BOOL;
  if ("NULL" in av && av.NULL) return null;
  if ("SS" in av && av.SS !== undefined) return av.SS.slice().sort();
  if ("NS" in av && av.NS !== undefined) return av.NS.slice().sort();
  if ("BS" in av && av.BS !== undefined)
    return av.BS.map((b) => Buffer.from(b).toString("base64")).sort();
  if ("L" in av && av.L !== undefined)
    return av.L.map(attributeValueToComparable);
  if ("M" in av && av.M !== undefined) {
    const out: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(av.M)) {
      if (child) out[key] = attributeValueToComparable(child);
    }
    return out;
  }
  throw new Error("unsupported AttributeValue");
}

function canonicalDecimalString(value: unknown): string {
  if (typeof value === "string") return value;
  if (typeof value === "bigint") return value.toString();
  if (typeof value === "number") {
    assert.ok(Number.isFinite(value), "DynamoDB N expectation must be finite");
    if (Number.isInteger(value)) {
      assert.ok(
        Number.isSafeInteger(value),
        "DynamoDB N integer expectations outside JS safe-integer range must be quoted decimal strings",
      );
    }
    const text = String(value);
    assert.ok(
      !/[eE]/.test(text),
      "DynamoDB N expectations using exponent notation must be quoted canonical decimal strings",
    );
    return text;
  }
  throw new Error(
    `expected DynamoDB canonical decimal string, got ${typeof value}`,
  );
}

function asStringArray(value: unknown): string[] {
  if (value === undefined || value === null) return [];
  if (Array.isArray(value)) return value.map((v) => String(v));
  return [String(value)];
}

function mapError(err: unknown): ErrorCode | "" {
  const codes = mapErrors(err);
  return codes[0] ?? "";
}

function mapErrors(err: unknown): ErrorCode[] {
  if (isTheorydbError(err)) {
    const maybeCodes = (err as { codes?: unknown }).codes;
    if (Array.isArray(maybeCodes)) {
      return maybeCodes.map((code) => String(code) as ErrorCode);
    }
    return [err.code];
  }
  return [];
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

async function captureError(
  fn: () => Promise<unknown>,
): Promise<unknown | undefined> {
  try {
    await fn();
    return undefined;
  } catch (err) {
    return err;
  }
}

async function captureResult<T>(
  fn: () => Promise<T>,
): Promise<{ value?: T; err?: unknown }> {
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

function marshalKey(
  model: DmsModel,
  key: Record<string, unknown>,
): Record<string, AttributeValue> {
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

async function recreateTable(
  ddb: DynamoDBClient,
  tableName: string,
  model: DmsModel,
): Promise<void> {
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

async function waitTableExists(
  ddb: DynamoDBClient,
  tableName: string,
): Promise<void> {
  for (let i = 0; i < 60; i++) {
    try {
      const resp = await ddb.send(
        new DescribeTableCommand({ TableName: tableName }),
      );
      if (resp.Table?.TableStatus === "ACTIVE") return;
    } catch (err) {
      if (!isResourceNotFound(err)) throw err;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`timeout waiting for table exists: ${tableName}`);
}

async function waitTableNotExists(
  ddb: DynamoDBClient,
  tableName: string,
): Promise<void> {
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

function createTableInput(
  tableName: string,
  model: DmsModel,
): CreateTableCommandInput {
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
    keySchema.push({
      AttributeName: model.keys.sort.attribute,
      KeyType: "RANGE",
    });
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
      indexKeySchema.push({
        AttributeName: idx.sort.attribute,
        KeyType: "RANGE",
      });
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
