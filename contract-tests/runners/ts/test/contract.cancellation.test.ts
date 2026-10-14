import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { TransactionCanceledException } from "@aws-sdk/client-dynamodb";
import { parse } from "yaml";

import { TheorydbClient } from "../../../../ts/src/client.js";
import { TheorydbError } from "../../../../ts/src/errors.js";
import { defineModel } from "../../../../ts/src/model.js";
import { createStatefulDynamoDBClient } from "../../../../ts/src/testkit/index.js";

interface CancellationFixtureReason {
  code?: string;
  message?: string;
}

interface CancellationFixtureCase {
  name: string;
  kind: string;
  reasons?: CancellationFixtureReason[];
  error_message?: string;
  expect: {
    code: string;
    reason_codes?: string[];
    cause_recoverable: boolean;
  };
}

interface CancellationFixture {
  name: string;
  cases: CancellationFixtureCase[];
}

const here = path.dirname(fileURLToPath(import.meta.url));
const fixturePath = path.join(
  here,
  "..",
  "..",
  "..",
  "scenarios",
  "cancellation",
  "01-transaction-cancellation.yml",
);

const userModel = defineModel({
  name: "User",
  table: { name: "users" },
  keys: {
    partition: { attribute: "PK", type: "S" },
    sort: { attribute: "SK", type: "S" },
  },
  attributes: [
    { attribute: "PK", type: "S", required: true, roles: ["pk"] },
    { attribute: "SK", type: "S", required: true, roles: ["sk"] },
    { attribute: "emailHash", type: "S" },
  ],
});

// The shared cancellation fixture is the cross-runtime contract: Go, TypeScript
// and Python each drive every case through their own stateful DynamoDB seam, so
// a divergent outcome is a parity defect. It needs no DynamoDB Local.
test("cancellation.transaction.taxonomy (ts runner)", async (t) => {
  const fixture = parse(
    readFileSync(fixturePath, "utf8"),
  ) as CancellationFixture;
  assert.ok(
    fixture.cases.length > 0,
    "fixture must declare cancellation cases",
  );

  for (const testCase of fixture.cases) {
    await t.test(testCase.name, async () => {
      const { fake, client: ddb } = createStatefulDynamoDBClient();
      const client = new TheorydbClient(ddb).register(userModel);

      switch (testCase.kind) {
        case "cancellation":
          fake.queueTransactionCancellation(
            cancellationReasons(testCase.reasons),
          );
          break;
        case "transport":
          fake.queueTransactionError(new Error(testCase.error_message ?? ""));
          break;
        default:
          assert.fail(`unknown fixture kind ${testCase.kind}`);
      }

      let thrown: unknown;
      let rejected = false;
      try {
        await client.transactWrite([
          {
            kind: "put",
            model: "User",
            item: { PK: "USER#1", SK: "PROFILE", emailHash: "hash_abc" },
          },
        ]);
      } catch (err) {
        rejected = true;
        thrown = err;
      }
      assert.ok(rejected, "transactWrite must reject");

      const code = thrown instanceof TheorydbError ? thrown.code : "";
      const reasonCodes =
        thrown instanceof TheorydbError
          ? thrown.cancellationReasons.map((reason) => reason.code)
          : [];
      const causeRecoverable = isRecoverableCancellation(thrown);

      assert.equal(code, testCase.expect.code, "mapped error code");
      assert.deepEqual(
        reasonCodes,
        testCase.expect.reason_codes ?? [],
        "ordered reason codes",
      );
      assert.equal(
        causeRecoverable,
        testCase.expect.cause_recoverable,
        "original cancellation recoverable",
      );
    });
  }
});

function cancellationReasons(
  reasons: readonly CancellationFixtureReason[] | undefined,
): Array<{ Code?: string; Message?: string }> {
  return (reasons ?? []).map((reason) => ({
    ...(reason.code !== undefined ? { Code: reason.code } : {}),
    ...(reason.message !== undefined ? { Message: reason.message } : {}),
  }));
}

// The raw AWS cancellation stays recoverable from the surfaced error either
// directly (an unclassified error passes through unchanged) or through the
// preserved cause of a mapped TheorydbError.
function isRecoverableCancellation(thrown: unknown): boolean {
  if (thrown instanceof TransactionCanceledException) return true;
  if (thrown instanceof TheorydbError) {
    return thrown.cause instanceof TransactionCanceledException;
  }
  return false;
}
