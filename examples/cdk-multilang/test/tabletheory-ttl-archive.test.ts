import assert from "node:assert/strict";
import test from "node:test";

import { App, Duration, Stack } from "aws-cdk-lib";
import { Template } from "aws-cdk-lib/assertions";
import * as dynamodb from "aws-cdk-lib/aws-dynamodb";

import {
  archiveExpiredRecords,
  type ArchiveWriter,
} from "../lambdas/archive/handler";
import { TableTheoryTtlArchive } from "../lib/tabletheory-ttl-archive";

test("TableTheoryTtlArchive synthesizes lifecycle, lambda, and stream mapping", () => {
  const app = new App();
  const stack = new Stack(app, "ArchiveStack");
  const table = new dynamodb.Table(stack, "EvidenceTable", {
    partitionKey: { name: "PK", type: dynamodb.AttributeType.STRING },
    sortKey: { name: "SK", type: dynamodb.AttributeType.STRING },
    stream: dynamodb.StreamViewType.NEW_AND_OLD_IMAGES,
    timeToLiveAttribute: "expires_at",
  });

  new TableTheoryTtlArchive(stack, "Archive", {
    archivePrefix: "evidence",
    batchSize: 500,
    expireAfter: Duration.days(730),
    glacierTransitionAfter: Duration.days(30),
    parallelizationFactor: 4,
    table,
    ttlAttributeName: "expires_at",
    uploadConcurrency: 40,
  });

  const template = Template.fromStack(stack);
  template.hasResourceProperties("AWS::S3::Bucket", {
    LifecycleConfiguration: {
      Rules: [
        {
          ExpirationInDays: 730,
          Prefix: "evidence/",
          Status: "Enabled",
          Transitions: [
            {
              StorageClass: "DEEP_ARCHIVE",
              TransitionInDays: 30,
            },
          ],
        },
      ],
    },
  });
  template.hasResourceProperties("AWS::Lambda::Function", {
    Environment: {
      Variables: {
        ARCHIVE_PREFIX: "evidence",
        ARCHIVE_UPLOAD_CONCURRENCY: "40",
        TTL_ATTRIBUTE_NAME: "expires_at",
      },
    },
    MemorySize: 1024,
    Timeout: 300,
  });
  template.hasResourceProperties("AWS::Lambda::EventSourceMapping", {
    BatchSize: 500,
    BisectBatchOnFunctionError: true,
    FunctionResponseTypes: ["ReportBatchItemFailures"],
    MaximumBatchingWindowInSeconds: 5,
    ParallelizationFactor: 4,
    StartingPosition: "LATEST",
  });
});

test("archiveExpiredRecords handles evidence-scale ttl batches with bounded concurrency", async () => {
  const records = Array.from({ length: 1000 }, (_, index) => ({
    eventID: `evt-${index}`,
    eventName: "REMOVE",
    userIdentity: {
      type: "Service",
      principalId: "dynamodb.amazonaws.com",
    },
    dynamodb: {
      ApproximateCreationDateTime: 1_742_688_000,
      Keys: { PK: { S: `merchant#${index}` } },
      OldImage: {
        expires_at: { N: "1742688000" },
        payload: { S: `snapshot-${index}` },
      },
    },
  }));

  let activeUploads = 0;
  let maxConcurrentUploads = 0;
  let uploadCount = 0;

  const writer: ArchiveWriter = {
    putObject: async () => {
      activeUploads += 1;
      maxConcurrentUploads = Math.max(maxConcurrentUploads, activeUploads);
      uploadCount += 1;
      await new Promise<void>((resolve) => setImmediate(resolve));
      activeUploads -= 1;
    },
  };

  const result = await archiveExpiredRecords(records, {
    archivePrefix: "evidence",
    bucketName: "archive-bucket",
    now: () => new Date("2026-03-23T00:00:00Z"),
    ttlAttributeName: "expires_at",
    uploadConcurrency: 32,
    writer,
  });

  assert.equal(result.archived, 1000);
  assert.equal(result.skipped, 0);
  assert.deepEqual(result.batchItemFailures, []);
  assert.equal(uploadCount, 1000);
  assert.ok(maxConcurrentUploads <= 32);
});

for (const view of [
  dynamodb.StreamViewType.KEYS_ONLY,
  dynamodb.StreamViewType.NEW_IMAGE,
]) {
  test(`TableTheoryTtlArchive rejects a ${view} stream view`, () => {
    const app = new App();
    const stack = new Stack(app, `Incompatible${view.replace(/_/g, "")}Stack`);
    const table = new dynamodb.Table(stack, "EvidenceTable", {
      partitionKey: { name: "PK", type: dynamodb.AttributeType.STRING },
      sortKey: { name: "SK", type: dynamodb.AttributeType.STRING },
      stream: view,
      timeToLiveAttribute: "expires_at",
    });

    assert.throws(
      () =>
        new TableTheoryTtlArchive(stack, "Archive", {
          table,
          ttlAttributeName: "expires_at",
        }),
      /NEW_AND_OLD_IMAGES/,
    );
  });
}

test("TableTheoryTtlArchive allows a table whose stream view is not knowable", () => {
  const app = new App();
  const stack = new Stack(app, "ImportedTableStack");
  const table = dynamodb.Table.fromTableAttributes(stack, "ImportedTable", {
    tableName: "imported-evidence",
    tableStreamArn:
      "arn:aws:dynamodb:us-east-1:123456789012:table/imported-evidence/stream/2026-01-01T00:00:00.000",
  });

  assert.doesNotThrow(() => {
    new TableTheoryTtlArchive(stack, "Archive", {
      table,
      ttlAttributeName: "expires_at",
    });
  });
});

test("archiveExpiredRecords fails closed when a TTL REMOVE lacks OldImage", async () => {
  let uploadCount = 0;
  const writer: ArchiveWriter = {
    putObject: async () => {
      uploadCount += 1;
    },
  };

  const result = await archiveExpiredRecords(
    [
      {
        eventID: "evt-missing-old-image",
        eventName: "REMOVE",
        userIdentity: {
          type: "Service",
          principalId: "dynamodb.amazonaws.com",
        },
        dynamodb: { Keys: { PK: { S: "merchant#1" } } },
      },
    ],
    {
      bucketName: "archive-bucket",
      now: () => new Date("2026-03-23T00:00:00Z"),
      ttlAttributeName: "expires_at",
      writer,
    },
  );

  assert.deepEqual(result.batchItemFailures, [
    { itemIdentifier: "evt-missing-old-image" },
  ]);
  assert.equal(result.archived, 0);
  assert.equal(result.skipped, 0);
  assert.equal(uploadCount, 0);
});

test("archiveExpiredRecords archives TTL REMOVE records with OldImage and skips the rest", async () => {
  const uploadedKeys: string[] = [];
  const writer: ArchiveWriter = {
    putObject: async (input) => {
      uploadedKeys.push(String(input.Key));
    },
  };

  const result = await archiveExpiredRecords(
    [
      {
        eventID: "evt-ttl",
        eventName: "REMOVE",
        userIdentity: {
          type: "Service",
          principalId: "dynamodb.amazonaws.com",
        },
        dynamodb: {
          Keys: { PK: { S: "merchant#1" } },
          OldImage: { expires_at: { N: "1742688000" } },
        },
      },
      {
        eventID: "evt-insert",
        eventName: "INSERT",
        userIdentity: {
          type: "Service",
          principalId: "dynamodb.amazonaws.com",
        },
        dynamodb: { Keys: { PK: { S: "merchant#2" } } },
      },
      {
        eventID: "evt-user-remove",
        eventName: "REMOVE",
        userIdentity: {
          type: "Service",
          principalId: "lambda.amazonaws.com",
        },
        dynamodb: {
          Keys: { PK: { S: "merchant#3" } },
          OldImage: { expires_at: { N: "1742688000" } },
        },
      },
    ],
    {
      bucketName: "archive-bucket",
      now: () => new Date("2026-03-23T00:00:00Z"),
      ttlAttributeName: "expires_at",
      writer,
    },
  );

  assert.equal(result.archived, 1);
  assert.equal(result.skipped, 2);
  assert.deepEqual(result.batchItemFailures, []);
  assert.deepEqual(uploadedKeys, ["ttl-archive/2026/03/23/evt-ttl.json"]);
});
