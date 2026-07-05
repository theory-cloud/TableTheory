// Node.js reader for the local cross-language no-drift check. It reads the
// item written by the Go writer against the same local DynamoDB table using the
// in-repo TypeScript runtime, and fails if the shape drifted.
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { TheorydbClient, defineModel } from '../../../ts/src/index.js';

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
  ],
});

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint: process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8020',
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

const db = new TheorydbClient(ddb).register(DemoItem);
const item = await db.get('DemoItem', { PK: 'demo#1', SK: 'v1' });
if (item.value !== 'shared-value' || item.lang !== 'go') {
  throw new Error(`node: drift detected: ${JSON.stringify(item)}`);
}
console.log(`node: read demo#1/v1 value=${String(item.value)} lang=${String(item.lang)}`);
