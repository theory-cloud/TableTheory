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
