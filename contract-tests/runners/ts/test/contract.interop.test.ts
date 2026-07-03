import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { DynamoDBClient } from "@aws-sdk/client-dynamodb";

import { loadModelsDir, loadScenariosDir } from "../src/load.js";
import { TheorydbDriver } from "../src/driver.js";
import { pingDynamo, runScenario } from "../src/runner.js";
import { defineModel } from "../../../../ts/src/model.js";
import type { Driver } from "../src/driver.js";
import type { Scenario } from "../src/types.js";

function contractRoot(): string {
  const __dirname = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(__dirname, "..", "..", ".."); // runners/ts/test -> contract-tests
}

test("cross-runtime interop scenarios (ts read phase)", async (t) => {
  if (process.env.CONTRACT_RUN_INTEROP !== "1") {
    t.skip("set CONTRACT_RUN_INTEROP=1 to run cross-runtime interop scenarios");
    return;
  }

  const root = contractRoot();
  const models = await loadModelsDir(path.join(root, "dms", "v0.1", "models"));
  const scenarios = await loadScenariosDir(
    path.join(root, "scenarios", "interop"),
  );
  assert.ok(models.size > 0);
  assert.ok(scenarios.length > 0);

  const endpoint = process.env.DYNAMODB_ENDPOINT ?? "http://localhost:8000";
  const ddb = new DynamoDBClient({
    region: process.env.AWS_REGION ?? "us-east-1",
    endpoint,
    credentials: {
      accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? "dummy",
      secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? "dummy",
    },
  });

  await pingDynamo(ddb);

  const compiled = Array.from(models.values()).map((m) => defineModel(m));
  const driver = new TheorydbDriver(ddb, compiled);

  for (const s of scenarios) {
    await t.test(s.name, async (st) => {
      const missing = missingCapabilities(s, driver);
      if (missing.length > 0) {
        st.skip(
          `scenario requires unsupported capabilities: ${missing.join(", ")}`,
        );
        return;
      }

      await runScenario({ ddb, driver, scenario: s, models });
    });
  }
});

function missingCapabilities(scenario: Scenario, driver: Driver): string[] {
  const supported = new Set(driver.capabilities());
  return (scenario.requires_capabilities ?? []).filter(
    (capability) => !supported.has(capability),
  );
}
