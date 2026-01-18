import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { loadModelsDir, loadScenariosDir } from "../src/load.js";
import { decodeCursor, encodeCursor } from "../../../../ts/src/cursor.js";

function contractRoot(): string {
  const __dirname = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(__dirname, "..", "..", ".."); // runners/ts/test -> contract-tests
}

test("loads DMS models + P0 scenarios", async () => {
  const root = contractRoot();
  const models = await loadModelsDir(path.join(root, "dms", "v0.1", "models"));
  assert.ok(models.has("User"));
  assert.ok(models.has("Order"));

  const scenarios = await loadScenariosDir(path.join(root, "scenarios", "p0"));
  assert.ok(scenarios.length >= 1);
});

test("golden cursor decodes to expected JSON", async () => {
  const root = contractRoot();
  const cursor = (await fs.readFile(path.join(root, "golden", "cursor", "cursor_v0.1_basic.cursor"), "utf8")).trim();
  const expectedJSON = (await fs.readFile(path.join(root, "golden", "cursor", "cursor_v0.1_basic.json"), "utf8")).trim();

  const decoded = Buffer.from(cursor, "base64url").toString("utf8");
  assert.equal(decoded, expectedJSON);

  const parsed = decodeCursor(cursor);
  assert.equal(encodeCursor(parsed), cursor);
});
