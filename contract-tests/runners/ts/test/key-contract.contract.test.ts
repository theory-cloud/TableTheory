import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  parseDerivedKeyContract,
  verifyDerivedKeyFixtures,
} from "../../../../ts/src/key-contract.js";

const keyContractsDir = fileURLToPath(
  new URL("../../../key-contracts", import.meta.url),
);

const fixturePaths = readdirSync(keyContractsDir, { withFileTypes: true })
  .filter((entry) => entry.isDirectory() && /^v\d+\.\d+$/.test(entry.name))
  .flatMap((entry) =>
    readdirSync(path.join(keyContractsDir, entry.name))
      .filter((name) => name.endsWith(".yml") || name.endsWith(".yaml"))
      .map((name) => path.join(keyContractsDir, entry.name, name)),
  )
  .sort();

assert.ok(fixturePaths.length > 0);

const contracts = fixturePaths.map((fixturePath) => {
  const contract = parseDerivedKeyContract(readFileSync(fixturePath, "utf8"));
  verifyDerivedKeyFixtures(contract);
  return contract;
});

const allKeys = new Set(
  contracts.flatMap((contract) => contract.derived_keys.map((key) => key.name)),
);

assert.ok(allKeys.has("CanonicalPolicyKey"));
assert.ok(allKeys.has("CanonicalBindingKey"));
assert.ok(allKeys.has("InterfaceScopeKey"));
assert.ok(allKeys.has("LowercaseLookupKey"));
