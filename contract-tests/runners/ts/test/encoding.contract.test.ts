import test from "node:test";
import assert from "node:assert/strict";

import { marshalScalar } from "../../../../ts/src/marshal.js";

test("empty string set encodes NULL (not empty SS)", () => {
  const av = marshalScalar({ attribute: "tags", type: "SS" }, []);
  assert.deepEqual(av, { NULL: true });
});

