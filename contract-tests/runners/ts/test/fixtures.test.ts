import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { loadModelsDir, loadScenariosDir } from "../src/load.js";
import { runScenario } from "../src/runner.js";
import { decodeCursor, encodeCursor } from "../../../../ts/src/cursor.js";
import { fromDynamoJson } from "../../../../ts/src/dynamo-json.js";
import type { Driver } from "../src/driver.js";
import type { DmsModel, Scenario, Step } from "../src/types.js";

function contractRoot(): string {
  const __dirname = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(__dirname, "..", "..", ".."); // runners/ts/test -> contract-tests
}

test("loads DMS models + P0 scenarios", async () => {
  const root = contractRoot();
  const models = await loadModelsDir(path.join(root, "dms", "v0.1", "models"));
  assert.ok(models.has("User"));
  assert.ok(models.has("Order"));
  assert.equal(models.get("ReleaseStateActual")?.write_policy?.mode, "mutable");
  assert.deepEqual(
    models.get("ReleaseStateActual")?.write_policy?.protected_attributes,
    ["pinnedReleaseId"],
  );
  assert.equal(
    models.get("ReleaseStateEvent")?.write_policy?.mode,
    "write_once",
  );

  const scenarios = await loadScenariosDir(path.join(root, "scenarios", "p0"));
  assert.ok(scenarios.length >= 1);
  assert.ok(scenarios.some((s) => s.name === "p0.release_state.write_policy"));
});

test("golden cursor corpus decodes to expected JSON", async () => {
  const root = contractRoot();
  const cursorDir = path.join(root, "golden", "cursor");
  const jsonFiles = (await fs.readdir(cursorDir))
    .filter((name) => name.startsWith("cursor_v0.1_") && name.endsWith(".json"))
    .sort();
  assert.ok(jsonFiles.length > 0);

  for (const jsonFile of jsonFiles) {
    const stem = jsonFile.replace(/\.json$/, "");
    const cursor = (
      await fs.readFile(path.join(cursorDir, `${stem}.cursor`), "utf8")
    ).trim();
    const expectedJSON = (
      await fs.readFile(path.join(cursorDir, jsonFile), "utf8")
    ).trim();
    const expected = JSON.parse(expectedJSON) as {
      lastKey: Record<string, unknown>;
      index?: string;
      sort?: "ASC" | "DESC";
    };

    const lastKey = Object.fromEntries(
      Object.entries(expected.lastKey).map(([key, value]) => [
        key,
        fromDynamoJson(value),
      ]),
    );
    assert.equal(encodeCursor({ ...expected, lastKey }), cursor);
    if (cursor === "") continue;

    const decoded = Buffer.from(cursor, "base64url").toString("utf8");
    assert.equal(decoded, expectedJSON);

    const parsed = decodeCursor(cursor);
    assert.equal(encodeCursor(parsed), cursor);
  }
});

test("interop item assertions fail closed on get errors without ok", async () => {
  await assert.rejects(
    () =>
      runScenario({
        ddb: {} as never,
        driver: stubDriver({
          get: async () => {
            throw new Error("missing seeded item");
          },
        }),
        scenario: scenarioWithReadStep({
          op: "get",
          key: { PK: "USER#fail-closed", SK: "PROFILE#fail-closed" },
          expect: {
            item_equals: {
              PK: "USER#fail-closed",
              SK: "PROFILE#fail-closed",
            },
          },
        }),
        models: new Map([[userModel.name, userModel]]),
      }),
    /expected successful operation for item assertions/,
  );
});

test("interop read assertions fail closed on query errors without ok", async () => {
  await assert.rejects(
    () =>
      runScenario({
        ddb: {} as never,
        driver: stubDriver({
          query: async () => {
            throw new Error("read failed");
          },
        }),
        scenario: scenarioWithReadStep({
          op: "query",
          query: {
            partition: {
              attribute: "PK",
              operator: "=",
              value: "USER#fail-closed",
            },
          },
          expect: {
            items_contains: [{ PK: "USER#fail-closed" }],
          },
        }),
        models: new Map([[userModel.name, userModel]]),
      }),
    /expected successful read for read assertions/,
  );
});

const userModel: DmsModel = {
  name: "User",
  table: { name: "users_contract" },
  keys: {
    partition: { attribute: "PK", type: "S" },
    sort: { attribute: "SK", type: "S" },
  },
  attributes: [
    { attribute: "PK", type: "S", required: true, roles: ["pk"] },
    { attribute: "SK", type: "S", required: true, roles: ["sk"] },
  ],
};

function scenarioWithReadStep(step: Step): Scenario {
  return {
    name: "unit.interop.fail_closed_assertions",
    dms_version: "0.1",
    requires_capabilities: [],
    model: userModel.name,
    table: { name: userModel.table.name },
    steps: [],
    seed_runtime: "go",
    seed_steps: [
      {
        op: "create",
        item: { PK: "unused", SK: "unused" },
        expect: { ok: true },
      },
    ],
    read_steps: [step],
  };
}

function stubDriver(overrides: Partial<Driver>): Driver {
  const fail = async (): Promise<never> => {
    throw new Error("unexpected stub driver call");
  };
  return {
    capabilities: () => [],
    create: fail,
    get: fail,
    update: fail,
    save: fail,
    delete: fail,
    query: fail,
    scan: fail,
    transitionAppendEvent: fail,
    validateProvenance: fail,
    ...overrides,
  };
}
