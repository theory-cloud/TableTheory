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
  if (isCancellationLike(thrown)) return true;
  if (thrown instanceof TheorydbError) {
    return isCancellationLike(thrown.cause);
  }
  return false;
}

// Recognizes a cancellation without relying on this module graph's constructor.
//
// This runner resolves its own `@aws-sdk/client-dynamodb` install, which need not
// be the copy the workspace client used. The exception classes in the installed
// SDK carry a Smithy `Symbol.hasInstance` that matches by shape, so `instanceof`
// already crosses graphs there; an older or partial graph has no such shim. The
// explicit name check is the rule `mapDynamoError` applies, so this asserts the
// production rule rather than whatever identity shim the installed SDK provides.
function isCancellationLike(value: unknown): boolean {
  if (value instanceof TransactionCanceledException) return true;
  return (
    typeof value === 'object' &&
    value !== null &&
    (value as { name?: unknown }).name === 'TransactionCanceledException'
  );
}

// A cancellation named and shaped exactly as a second SDK graph produces one,
// but built from a constructor no SDK identity shim knows. The control asserts
// `instanceof` is false for it, so the recognition below cannot be an identity
// check wearing a different hat.
class ForeignSdkCancellation extends Error {
  readonly $metadata = {};
  readonly CancellationReasons: readonly unknown[] = [];

  constructor() {
    super('Transaction canceled.');
    this.name = 'TransactionCanceledException';
  }
}

test("cancellation.recognition.survives-a-second-sdk-graph (ts runner)", () => {
  const foreign = new ForeignSdkCancellation();
  assert.equal(
    foreign instanceof TransactionCanceledException,
    false,
    "the control must exercise the non-instanceof path",
  );
  assert.equal(isCancellationLike(foreign), true);
  assert.equal(isRecoverableCancellation(foreign), true);

  const mapped = new TheorydbError("ErrTransactionCanceled", "canceled", {
    cause: foreign,
  });
  assert.equal(isRecoverableCancellation(mapped), true);

  const unrelated = new Error("connection reset");
  assert.equal(isCancellationLike(unrelated), false);
  assert.equal(isRecoverableCancellation(unrelated), false);

  const renamed = new Error("slow down");
  renamed.name = "ThrottlingException";
  assert.equal(isCancellationLike(renamed), false);
  assert.equal(isRecoverableCancellation(renamed), false);

  const unmapped = new TheorydbError("ErrInvalidModel", "bad model", {
    cause: unrelated,
  });
  assert.equal(isRecoverableCancellation(unmapped), false);
});
