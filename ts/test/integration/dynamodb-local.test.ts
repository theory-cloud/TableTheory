import assert from 'node:assert/strict';
import { DynamoDBClient, ListTablesCommand } from '@aws-sdk/client-dynamodb';

const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';
const skipIntegration =
  process.env.SKIP_INTEGRATION === 'true' ||
  process.env.SKIP_INTEGRATION === '1';

const client = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

try {
  const resp = await client.send(new ListTablesCommand({ Limit: 1 }));
  assert.ok(resp.TableNames !== undefined);
} catch (err) {
  if (skipIntegration) {
    console.warn(
      `Skipping DynamoDB Local integration test (SKIP_INTEGRATION set; endpoint: ${endpoint})`,
    );
    process.exit(0);
  }
  throw err;
} finally {
  client.destroy();
}
