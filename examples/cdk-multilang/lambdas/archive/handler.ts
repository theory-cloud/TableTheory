import {
  PutObjectCommand,
  S3Client,
  type PutObjectCommandInput,
} from "@aws-sdk/client-s3";

type StreamImage = Record<string, unknown>;

export interface StreamRecord {
  eventID?: string;
  eventName?: string;
  eventSourceARN?: string;
  userIdentity?: {
    type?: string;
    principalId?: string;
  };
  dynamodb?: {
    ApproximateCreationDateTime?: number;
    Keys?: StreamImage;
    OldImage?: StreamImage;
  };
}

export interface ArchiveWriter {
  putObject(input: PutObjectCommandInput): Promise<void>;
}

export interface ArchiveOptions {
  archivePrefix?: string;
  bucketName: string;
  now?: () => Date;
  ttlAttributeName: string;
  uploadConcurrency?: number;
  writer: ArchiveWriter;
}

export interface ArchiveResult {
  archived: number;
  batchItemFailures: Array<{ itemIdentifier: string }>;
  skipped: number;
}

export function isTtlExpiredRecord(record: StreamRecord): boolean {
  return (
    record.eventName === "REMOVE" &&
    record.userIdentity?.type === "Service" &&
    record.userIdentity?.principalId === "dynamodb.amazonaws.com" &&
    record.dynamodb?.OldImage !== undefined
  );
}

export async function archiveExpiredRecords(
  records: readonly StreamRecord[],
  opts: ArchiveOptions,
): Promise<ArchiveResult> {
  const ttlRecords = records.filter(isTtlExpiredRecord);
  const now = opts.now ?? (() => new Date());
  const prefix = normalizePrefix(opts.archivePrefix ?? "ttl-archive");
  const uploadConcurrency = Math.max(
    1,
    Math.floor(opts.uploadConcurrency ?? 25),
  );
  const batchItemFailures: Array<{ itemIdentifier: string }> = [];

  await mapWithConcurrency(
    ttlRecords,
    uploadConcurrency,
    async (record, index) => {
      try {
        const archivedAt = now();
        await opts.writer.putObject({
          Bucket: opts.bucketName,
          Key: buildObjectKey(prefix, archivedAt, record, index),
          ContentType: "application/json",
          Body: JSON.stringify({
            archived_at: archivedAt.toISOString(),
            approximate_creation_date_time:
              record.dynamodb?.ApproximateCreationDateTime ?? null,
            event_id: record.eventID ?? null,
            event_source_arn: record.eventSourceARN ?? null,
            keys: record.dynamodb?.Keys ?? null,
            old_image: record.dynamodb?.OldImage ?? null,
            ttl_attribute: opts.ttlAttributeName,
            ttl_value:
              record.dynamodb?.OldImage?.[opts.ttlAttributeName] ?? null,
          }),
        });
      } catch (error) {
        if (record.eventID) {
          batchItemFailures.push({ itemIdentifier: record.eventID });
          return;
        }
        throw error;
      }
    },
  );

  return {
    archived: ttlRecords.length - batchItemFailures.length,
    batchItemFailures,
    skipped: records.length - ttlRecords.length,
  };
}

export async function handler(event: {
  Records?: StreamRecord[];
}): Promise<{ batchItemFailures: Array<{ itemIdentifier: string }> }> {
  const bucketName = requiredEnv("ARCHIVE_BUCKET_NAME");
  const ttlAttributeName = requiredEnv("TTL_ATTRIBUTE_NAME");
  const archivePrefix = process.env.ARCHIVE_PREFIX ?? "ttl-archive";
  const uploadConcurrency = parsePositiveInt(
    process.env.ARCHIVE_UPLOAD_CONCURRENCY,
    25,
  );

  const client = new S3Client({});
  const writer: ArchiveWriter = {
    putObject: async (input) => {
      await client.send(new PutObjectCommand(input));
    },
  };

  const result = await archiveExpiredRecords(event.Records ?? [], {
    archivePrefix,
    bucketName,
    ttlAttributeName,
    uploadConcurrency,
    writer,
  });

  return { batchItemFailures: result.batchItemFailures };
}

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`missing required environment variable ${name}`);
  }
  return value;
}

function parsePositiveInt(value: string | undefined, fallback: number): number {
  if (!value) return fallback;
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return parsed;
}

function normalizePrefix(prefix: string): string {
  return prefix.replace(/^\/+|\/+$/g, "");
}

function buildObjectKey(
  prefix: string,
  archivedAt: Date,
  record: StreamRecord,
  index: number,
): string {
  const year = archivedAt.getUTCFullYear();
  const month = String(archivedAt.getUTCMonth() + 1).padStart(2, "0");
  const day = String(archivedAt.getUTCDate()).padStart(2, "0");
  const eventID = record.eventID ?? `record-${index}`;
  return `${prefix}/${year}/${month}/${day}/${eventID}.json`;
}

async function mapWithConcurrency<T>(
  items: readonly T[],
  concurrency: number,
  fn: (item: T, index: number) => Promise<void>,
): Promise<void> {
  const workers = Array.from(
    { length: Math.min(concurrency, Math.max(1, items.length)) },
    async (_, workerIndex) => {
      for (
        let index = workerIndex;
        index < items.length;
        index += concurrency
      ) {
        await fn(items[index], index);
      }
    },
  );
  await Promise.all(workers);
}
