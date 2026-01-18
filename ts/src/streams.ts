import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { fromDynamoJson } from './dynamo-json.js';
import { TheorydbError } from './errors.js';
import type { Model } from './model.js';
import { unmarshalItem } from './marshal.js';

export interface StreamRecord {
  dynamodb?: {
    Keys?: Record<string, unknown>;
    NewImage?: Record<string, unknown>;
    OldImage?: Record<string, unknown>;
  };
}

export function unmarshalStreamImage(
  model: Model,
  image: Record<string, unknown>,
): Record<string, unknown> {
  const attrs: Record<string, AttributeValue> = {};

  for (const [name, avJson] of Object.entries(image)) {
    attrs[name] = fromDynamoJson(avJson);
  }

  return unmarshalItem(model, attrs);
}

export function unmarshalStreamRecord(
  model: Model,
  record: StreamRecord,
): {
  keys?: Record<string, unknown>;
  newImage?: Record<string, unknown>;
  oldImage?: Record<string, unknown>;
} {
  const ddb = record.dynamodb;
  if (!ddb)
    throw new TheorydbError(
      'ErrInvalidModel',
      'Stream record missing dynamodb field',
    );

  const out: {
    keys?: Record<string, unknown>;
    newImage?: Record<string, unknown>;
    oldImage?: Record<string, unknown>;
  } = {};

  if (ddb.Keys) out.keys = unmarshalStreamImage(model, ddb.Keys);
  if (ddb.NewImage) out.newImage = unmarshalStreamImage(model, ddb.NewImage);
  if (ddb.OldImage) out.oldImage = unmarshalStreamImage(model, ddb.OldImage);

  return out;
}
