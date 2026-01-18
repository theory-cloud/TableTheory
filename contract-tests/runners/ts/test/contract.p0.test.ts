import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { DynamoDBClient } from "@aws-sdk/client-dynamodb";

import { loadModelsDir, loadScenariosDir } from "../src/load.js";
import { TheorydbDriver } from "../src/driver.js";
import { pingDynamo, runScenario } from "../src/runner.js";
import { defineModel } from "../../../../ts/src/model.js";

function contractRoot(): string {
  const __dirname = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(__dirname, "..", "..", ".."); // runners/ts/test -> contract-tests
}

test("P0 contract scenarios (ts runner)", async (t) => {
  const root = contractRoot();
  const models = await loadModelsDir(path.join(root, "dms", "v0.1", "models"));
  const scenarios = await loadScenariosDir(path.join(root, "scenarios", "p0"));
  assert.ok(models.size > 0);
  assert.ok(scenarios.length > 0);

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

  const compiled = Array.from(models.values()).map((m) => defineModel(m));
  const driver = new TheorydbDriver(ddb, compiled);

  for (const s of scenarios) {
    const model = models.get(s.model);
    assert.ok(model, `unknown model: ${s.model}`);
    await runScenario({ ddb, driver, scenario: s, model });
  }
});
