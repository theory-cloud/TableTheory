// Node.js reader for the local cross-language no-drift check. It reads the
// item written by the Go writer against the same local DynamoDB table using the
// in-repo TypeScript runtime, decrypts the encrypted secret through the local
// KMS, and fails if the shape or the plaintext drifted.
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { KMSClient } from '@aws-sdk/client-kms';

import {
  AwsKmsEncryptionProvider,
  TheorydbClient,
  defineModel,
} from '../../../ts/src/index.js';

const kmsKeyArn = process.env.KMS_KEY_ARN ?? '';
if (kmsKeyArn === '') {
  throw new Error('KMS_KEY_ARN is required for the encrypted local variant');
}

const DemoItem = defineModel({
  name: 'DemoItem',
  table: { name: process.env.DEMO_TABLE_NAME ?? 'demo_multilang_local' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'value', type: 'S' },
    { attribute: 'lang', type: 'S' },
    { attribute: 'secret', type: 'S', encryption: { v: 1 } },
  ],
});

const credentials = {
  accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
  secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
};

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint: process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8020',
  credentials,
});

const kms = new KMSClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint:
    process.env.KMS_ENDPOINT ??
    process.env.AWS_ENDPOINT_URL_KMS ??
    'http://localhost:8021',
  credentials,
});

const encryption = new AwsKmsEncryptionProvider(kms, { keyArn: kmsKeyArn });
const db = new TheorydbClient(ddb, { encryption }).register(DemoItem);
const item = await db.get('DemoItem', { PK: 'demo#1', SK: 'v1' });
if (item.value !== 'shared-value' || item.lang !== 'go') {
  throw new Error(`node: drift detected: ${JSON.stringify(item)}`);
}
if (item.secret !== 'shared-secret-v1') {
  throw new Error(`node: secret did not decrypt: ${JSON.stringify(item.secret)}`);
}
console.log(
  `node: read demo#1/v1 value=${String(item.value)} lang=${String(item.lang)} secret=<decrypted>`,
);
