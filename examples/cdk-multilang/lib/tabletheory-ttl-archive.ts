import path from "node:path";

import { Duration, RemovalPolicy } from "aws-cdk-lib";
import * as dynamodb from "aws-cdk-lib/aws-dynamodb";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as lambdaEventSources from "aws-cdk-lib/aws-lambda-event-sources";
import { NodejsFunction } from "aws-cdk-lib/aws-lambda-nodejs";
import * as s3 from "aws-cdk-lib/aws-s3";
import { Construct } from "constructs";

export interface TableTheoryTtlArchiveProps {
  archiveBucket?: s3.IBucket;
  archivePrefix?: string;
  archiveStorageClass?: s3.StorageClass;
  autoDeleteObjects?: boolean;
  batchSize?: number;
  bisectBatchOnError?: boolean;
  expireAfter?: Duration;
  glacierTransitionAfter?: Duration;
  lambdaMemorySize?: number;
  lambdaTimeout?: Duration;
  maxBatchingWindow?: Duration;
  parallelizationFactor?: number;
  removalPolicy?: RemovalPolicy;
  retryAttempts?: number;
  table: dynamodb.ITable;
  ttlAttributeName: string;
  uploadConcurrency?: number;
}

export class TableTheoryTtlArchive extends Construct {
  readonly archiveBucket: s3.IBucket;
  readonly archiverFunction: lambda.Function;

  constructor(scope: Construct, id: string, props: TableTheoryTtlArchiveProps) {
    super(scope, id);

    if (!props.table.tableStreamArn) {
      throw new Error(
        "TableTheoryTtlArchive requires a table with DynamoDB Streams enabled",
      );
    }

    const archivePrefix = normalizePrefix(props.archivePrefix ?? "ttl-archive");
    const archiveBucket =
      props.archiveBucket ??
      new s3.Bucket(this, "ArchiveBucket", {
        autoDeleteObjects: props.autoDeleteObjects ?? false,
        encryption: s3.BucketEncryption.S3_MANAGED,
        lifecycleRules: [
          {
            enabled: true,
            ...(archivePrefix ? { prefix: `${archivePrefix}/` } : {}),
            ...(props.glacierTransitionAfter
              ? {
                  transitions: [
                    {
                      storageClass:
                        props.archiveStorageClass ??
                        s3.StorageClass.DEEP_ARCHIVE,
                      transitionAfter: props.glacierTransitionAfter,
                    },
                  ],
                }
              : {}),
            ...(props.expireAfter ? { expiration: props.expireAfter } : {}),
          },
        ],
        removalPolicy: props.removalPolicy ?? RemovalPolicy.RETAIN,
      });

    const entry = path.resolve(__dirname, "../lambdas/archive/handler.ts");
    const archiverFunction = new NodejsFunction(this, "ArchiverFunction", {
      bundling: {
        target: "node24",
      },
      entry,
      environment: {
        ARCHIVE_BUCKET_NAME: archiveBucket.bucketName,
        ARCHIVE_PREFIX: archivePrefix,
        ARCHIVE_UPLOAD_CONCURRENCY: String(props.uploadConcurrency ?? 25),
        TTL_ATTRIBUTE_NAME: props.ttlAttributeName,
      },
      handler: "handler",
      memorySize: props.lambdaMemorySize ?? 1024,
      runtime: lambda.Runtime.NODEJS_24_X,
      timeout: props.lambdaTimeout ?? Duration.minutes(5),
    });

    props.table.grantStreamRead(archiverFunction);
    archiveBucket.grantPut(archiverFunction);

    archiverFunction.addEventSource(
      new lambdaEventSources.DynamoEventSource(props.table, {
        batchSize: props.batchSize ?? 1000,
        bisectBatchOnError: props.bisectBatchOnError ?? true,
        maxBatchingWindow: props.maxBatchingWindow ?? Duration.seconds(5),
        parallelizationFactor: props.parallelizationFactor ?? 10,
        retryAttempts: props.retryAttempts ?? 3,
        reportBatchItemFailures: true,
        startingPosition: lambda.StartingPosition.LATEST,
      }),
    );

    this.archiveBucket = archiveBucket;
    this.archiverFunction = archiverFunction;
  }
}

function normalizePrefix(prefix: string): string {
  return prefix.replace(/^\/+|\/+$/g, "");
}
