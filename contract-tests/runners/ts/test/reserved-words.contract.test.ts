import test from "node:test";
import assert from "node:assert/strict";

import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
} from "@aws-sdk/client-dynamodb";

import { defineModel } from "../../../../ts/src/model.js";
import { TheorydbDriver } from "../src/driver.js";
import { pingDynamo } from "../src/runner.js";

async function recreateTable(ddb: DynamoDBClient, tableName: string): Promise<void> {
  try {
    await ddb.send(new DeleteTableCommand({ TableName: tableName }));
  } catch (err) {
    if (!isResourceNotFound(err)) throw err;
  }
  await waitTableNotExists(ddb, tableName);
  await ddb.send(
    new CreateTableCommand({
      TableName: tableName,
      BillingMode: "PAY_PER_REQUEST",
      AttributeDefinitions: [
        { AttributeName: "PK", AttributeType: "S" },
        { AttributeName: "SK", AttributeType: "S" },
      ],
      KeySchema: [
        { AttributeName: "PK", KeyType: "HASH" },
        { AttributeName: "SK", KeyType: "RANGE" },
      ],
    }),
  );
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
    await new Promise((r) => setTimeout(r, 250));
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
    await new Promise((r) => setTimeout(r, 150));
  }
}

test("reserved word update escapes attribute names", async (t) => {
  const endpoint = process.env.DYNAMODB_ENDPOINT ?? "http://localhost:8000";
  const skipIntegration = process.env.SKIP_INTEGRATION === "true" || process.env.SKIP_INTEGRATION === "1";
  const ddb = new DynamoDBClient({
    region: process.env.AWS_REGION ?? "us-east-1",
    endpoint,
    credentials: {
      accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? "dummy",
      secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? "dummy",
    },
  });

  try {
    await pingDynamo(ddb);
  } catch (err) {
    if (skipIntegration) {
      t.skip(`DynamoDB Local not reachable (SKIP_INTEGRATION set; endpoint: ${endpoint})`);
      return;
    }
    throw err;
  }

  const model = defineModel({
    name: "Reserved",
    table: { name: "reserved_words_contract" },
    keys: {
      partition: { attribute: "PK", type: "S" },
      sort: { attribute: "SK", type: "S" },
    },
    attributes: [
      { attribute: "PK", type: "S", roles: ["pk"] },
      { attribute: "SK", type: "S", roles: ["sk"] },
      { attribute: "name", type: "S", optional: true },
      { attribute: "createdAt", type: "S", roles: ["created_at"] },
      { attribute: "updatedAt", type: "S", roles: ["updated_at"] },
      { attribute: "version", type: "N", roles: ["version"] },
    ],
  });

  await recreateTable(ddb, model.tableName);

  const driver = new TheorydbDriver(ddb, [model]);

  await driver.create("Reserved", { PK: "A", SK: "B", name: "v0", version: 0 }, {});
  await driver.update("Reserved", { PK: "A", SK: "B", name: "v1", version: 0 }, ["name"]);

  const got = await driver.get("Reserved", { PK: "A", SK: "B" });
  assert.equal(got.name, "v1");
  assert.equal(got.version, 1);
});

