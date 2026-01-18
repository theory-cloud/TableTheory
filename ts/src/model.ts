import { TheorydbError } from './errors.js';

export type ScalarType =
  | 'S'
  | 'N'
  | 'B'
  | 'BOOL'
  | 'NULL'
  | 'M'
  | 'L'
  | 'SS'
  | 'NS'
  | 'BS';
export type KeyType = 'S' | 'N' | 'B';

export interface ValueConverter {
  toDynamoValue(value: unknown): unknown;
  fromDynamoValue(value: unknown): unknown;
}

export interface KeySchema {
  attribute: string;
  type: KeyType;
}

export interface AttributeSchema {
  attribute: string;
  type: ScalarType;
  required?: boolean;
  optional?: boolean;
  omit_empty?: boolean;
  json?: boolean;
  binary?: boolean;
  format?: string;
  roles?: string[];
  encryption?: unknown;
  converter?: ValueConverter;
}

export interface IndexSchema {
  name: string;
  type: 'GSI' | 'LSI';
  partition: KeySchema;
  sort?: KeySchema;
  projection?: { type: 'ALL' | 'KEYS_ONLY' | 'INCLUDE'; fields?: string[] };
}

export interface ModelSchema {
  name: string;
  table: { name: string };
  naming?: { convention?: 'camelCase' | 'snake_case' };
  keys: {
    partition: KeySchema;
    sort?: KeySchema;
  };
  attributes: AttributeSchema[];
  indexes?: IndexSchema[];
}

export interface ModelRoles {
  pk: string;
  sk?: string;
  createdAt?: string;
  updatedAt?: string;
  version?: string;
  ttl?: string;
}

export interface Model {
  readonly name: string;
  readonly tableName: string;
  readonly schema: Readonly<ModelSchema>;
  readonly attributes: ReadonlyMap<string, Readonly<AttributeSchema>>;
  readonly indexes: ReadonlyMap<string, Readonly<IndexSchema>>;
  readonly roles: Readonly<ModelRoles>;
}

export function defineModel(schema: ModelSchema): Model {
  validateModelSchema(schema);

  const attributes = new Map<string, AttributeSchema>();
  for (const attr of schema.attributes) {
    attributes.set(attr.attribute, attr);
  }

  const indexes = new Map<string, IndexSchema>();
  for (const idx of schema.indexes ?? []) {
    indexes.set(idx.name, idx);
  }

  return {
    name: schema.name,
    tableName: schema.table.name,
    schema,
    attributes,
    indexes,
    roles: resolveRoles(schema),
  };
}

function validateModelSchema(schema: ModelSchema): void {
  if (!schema?.name)
    throw new TheorydbError('ErrInvalidModel', 'Model name is required');
  if (!schema?.table?.name)
    throw new TheorydbError('ErrInvalidModel', 'Model table.name is required');
  if (!schema?.keys?.partition?.attribute) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Model keys.partition.attribute is required',
    );
  }
  if (!schema?.keys?.partition?.type) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Model keys.partition.type is required',
    );
  }
  if (!Array.isArray(schema.attributes) || schema.attributes.length === 0) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Model attributes[] is required',
    );
  }

  const attributeNames = new Set<string>();
  for (const attr of schema.attributes) {
    if (!attr?.attribute)
      throw new TheorydbError(
        'ErrInvalidModel',
        'Attribute attribute name is required',
      );
    if (attr.json && attr.type !== 'S') {
      throw new TheorydbError(
        'ErrInvalidModel',
        `json attributes must be type S: ${attr.attribute}`,
      );
    }
    if (attr.binary && attr.type !== 'B') {
      throw new TheorydbError(
        'ErrInvalidModel',
        `binary attributes must be type B: ${attr.attribute}`,
      );
    }
    if (attr.json && attr.binary) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `attribute cannot be both json and binary: ${attr.attribute}`,
      );
    }
    if (attributeNames.has(attr.attribute)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Duplicate attribute: ${attr.attribute}`,
      );
    }
    attributeNames.add(attr.attribute);
  }

  const pk = schema.keys.partition.attribute;
  const pkAttr = schema.attributes.find((a) => a.attribute === pk);
  if (!pkAttr)
    throw new TheorydbError(
      'ErrInvalidModel',
      `Partition key attribute missing: ${pk}`,
    );
  if (pkAttr.type !== schema.keys.partition.type) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Partition key type mismatch: ${pk}`,
    );
  }
  if (pkAttr.encryption) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Encrypted fields cannot be primary keys: ${pk}`,
    );
  }

  if (schema.keys.sort) {
    const sk = schema.keys.sort.attribute;
    const skAttr = schema.attributes.find((a) => a.attribute === sk);
    if (!skAttr)
      throw new TheorydbError(
        'ErrInvalidModel',
        `Sort key attribute missing: ${sk}`,
      );
    if (skAttr.type !== schema.keys.sort.type) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Sort key type mismatch: ${sk}`,
      );
    }
    if (skAttr.encryption) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Encrypted fields cannot be primary keys: ${sk}`,
      );
    }
  }

  const indexNames = new Set<string>();
  for (const idx of schema.indexes ?? []) {
    if (!idx?.name)
      throw new TheorydbError('ErrInvalidModel', 'Index name is required');
    if (indexNames.has(idx.name))
      throw new TheorydbError(
        'ErrInvalidModel',
        `Duplicate index: ${idx.name}`,
      );
    indexNames.add(idx.name);

    const pkAttrName = idx.partition.attribute;
    const pkIndexAttr = schema.attributes.find(
      (a) => a.attribute === pkAttrName,
    );
    if (!pkIndexAttr)
      throw new TheorydbError(
        'ErrInvalidModel',
        `Index key attribute missing: ${pkAttrName}`,
      );
    if (pkIndexAttr.type !== idx.partition.type) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Index key type mismatch: ${pkAttrName}`,
      );
    }
    if (pkIndexAttr.encryption) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Encrypted fields cannot be index keys: ${pkAttrName}`,
      );
    }

    if (idx.sort) {
      const skAttrName = idx.sort.attribute;
      const skIndexAttr = schema.attributes.find(
        (a) => a.attribute === skAttrName,
      );
      if (!skIndexAttr)
        throw new TheorydbError(
          'ErrInvalidModel',
          `Index sort key missing: ${skAttrName}`,
        );
      if (skIndexAttr.type !== idx.sort.type) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Index sort key type mismatch: ${skAttrName}`,
        );
      }
      if (skIndexAttr.encryption) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Encrypted fields cannot be index keys: ${skAttrName}`,
        );
      }
    }
  }
}

function resolveRoles(schema: ModelSchema): ModelRoles {
  const roles: ModelRoles = { pk: schema.keys.partition.attribute };
  if (schema.keys.sort) roles.sk = schema.keys.sort.attribute;

  for (const attr of schema.attributes) {
    for (const role of attr.roles ?? []) {
      switch (role) {
        case 'pk':
          roles.pk = attr.attribute;
          break;
        case 'sk':
          roles.sk = attr.attribute;
          break;
        case 'created_at':
          roles.createdAt = attr.attribute;
          break;
        case 'updated_at':
          roles.updatedAt = attr.attribute;
          break;
        case 'version':
          roles.version = attr.attribute;
          break;
        case 'ttl':
          roles.ttl = attr.attribute;
          break;
        default:
          break;
      }
    }
  }

  return roles;
}
