import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import {
  parseDerivedKeyContract,
  verifyDerivedKeyFixtures,
} from "../../../../ts/src/key-contract.js";

const fixturePath = fileURLToPath(
  new URL(
    "../../../key-contracts/v0.1/theorymcp-derived-keys.yml",
    import.meta.url,
  ),
);

const contract = parseDerivedKeyContract(readFileSync(fixturePath, "utf8"));
verifyDerivedKeyFixtures(contract);

assert.ok(
  contract.derived_keys.some((key) => key.name === "CanonicalPolicyKey"),
);
assert.ok(
  contract.derived_keys.some((key) => key.name === "CanonicalBindingKey"),
);
assert.ok(
  contract.derived_keys.some((key) => key.name === "InterfaceScopeKey"),
);
