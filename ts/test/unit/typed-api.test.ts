import assert from 'node:assert/strict';

import { PutItemCommand, type DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '../../src/client.js';
import { defineModel, type ModelItem } from '../../src/model.js';
import { unsafeOperator } from '../../src/query.js';

const TypedUser = defineModel({
  name: 'TypedUser',
  table: { name: 'typed_users' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'age', type: 'N' },
    { attribute: 'nickname', type: 'S', optional: true },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

type TypedUserItem = ModelItem<typeof TypedUser>;

class StubDdb {
  sent: unknown[] = [];

  async send(cmd: unknown): Promise<unknown> {
    this.sent.push(cmd);
    return {};
  }
}

const goodItem: TypedUserItem = { PK: 'TENANT#1', SK: 'USER#1', age: 42 };
assert.equal(goodItem.age, 42);

{
  const ddb = new StubDdb();
  const users = new TheorydbClient(ddb as unknown as DynamoDBClient).model(
    TypedUser,
  );
  await users.create({ PK: 'TENANT#1', SK: 'USER#1', age: 42 });
  const cmd = ddb.sent[0];
  assert.ok(cmd instanceof PutItemCommand);
  assert.equal(cmd.input.TableName, 'typed_users');
}

async function typeAssertions(client: TheorydbClient): Promise<void> {
  const users = client.model(TypedUser);
  await users.create({ PK: 'TENANT#1', SK: 'USER#1', age: 42 });
  await users.update({ PK: 'TENANT#1', SK: 'USER#1', age: 43 }, ['age']);

  const item = await users.get({ PK: 'TENANT#1', SK: 'USER#1' });
  item.age.toFixed();
  item.nickname?.toUpperCase();

  await users.query().partitionKey('TENANT#1').filter('age', '>', 21).page();
  await users
    .query()
    .partitionKey('TENANT#1')
    .filter('age', unsafeOperator('CUSTOM_RUNTIME_OPERATOR'), 21)
    .page();

  const untyped = client.model('TypedUser');
  await untyped.create({ intentionally: 'string lookup stays untyped' });

  // @ts-expect-error missing required SK and age fields
  await users.create({ PK: 'TENANT#1' });
  // @ts-expect-error age is inferred as a number
  await users.create({ PK: 'TENANT#1', SK: 'USER#1', age: '42' });
  // @ts-expect-error unknown fields are rejected for typed model handles
  await users.create({ PK: 'TENANT#1', SK: 'USER#1', age: 42, nope: true });
  // @ts-expect-error update field names are tied to the inferred item shape
  await users.update({ PK: 'TENANT#1', SK: 'USER#1', age: 43 }, ['missing']);
  // @ts-expect-error unknown operator literals require unsafeOperator(...)
  users.query().partitionKey('TENANT#1').filter('age', 'BOGUS', 21);
}

void typeAssertions;
