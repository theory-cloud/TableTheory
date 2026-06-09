import YAML from 'yaml';

import { TheorydbError } from './errors.js';

export type KeyContractInputValue =
  | string
  | number
  | boolean
  | null
  | undefined;
export type KeyContractTransform = 'trim' | 'wildcard_empty';

export interface DerivedKeyContract {
  tabletheory_model_contract_version: '0.1';
  namespace?: string;
  dms_version?: string;
  models?: DerivedKeyModelContract[];
  derived_keys: DerivedKeyDefinition[];
}

export interface DerivedKeyModelContract {
  name: string;
  dms_model?: string;
  table?: string;
  derived_keys?: string[];
}

export interface DerivedKeyDefinition {
  name: string;
  helper?: string;
  description?: string;
  join: string;
  inputs?: DerivedKeyInput[];
  segments: DerivedKeySegment[];
  fixtures?: DerivedKeyFixture[];
}

export interface DerivedKeyInput {
  name: string;
  ts_name?: string;
  type?: 'string' | 'number' | 'boolean' | 'scalar';
  optional?: boolean;
}

export interface DerivedKeySegment {
  name?: string;
  prefix?: string;
  suffix?: string;
  literal?: string;
  value?: DerivedKeyValueSource;
  transforms?: KeyContractTransform[];
  default?: string;
  optional?: boolean;
  omit_when?: DerivedKeyOmitWhen;
}

export interface DerivedKeyValueSource {
  input?: string;
  literal?: string;
}

export interface DerivedKeyOmitWhen {
  empty?: boolean;
  default?: boolean;
  values?: string[];
}

export interface DerivedKeyFixture {
  name: string;
  description?: string;
  input: Record<string, KeyContractInputValue>;
  expect: string;
}

export function parseDerivedKeyContract(raw: string): DerivedKeyContract {
  let parsed: unknown;
  try {
    parsed = YAML.parse(raw) as unknown;
  } catch (err) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Invalid key contract YAML/JSON',
      {
        cause: err,
      },
    );
  }
  validateDerivedKeyContract(parsed);
  return parsed;
}

export function evaluateDerivedKey(
  contract: DerivedKeyContract,
  name: string,
  input: Record<string, KeyContractInputValue> = {},
): string {
  validateDerivedKeyContract(contract);
  const key = contract.derived_keys.find(
    (candidate) => candidate.name === name,
  );
  if (!key) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Derived key not found: ${name}`,
    );
  }
  return evaluateDerivedKeyDefinition(key, input);
}

export function evaluateDerivedKeyDefinition(
  key: DerivedKeyDefinition,
  input: Record<string, KeyContractInputValue> = {},
): string {
  validateDerivedKeyDefinition(key);
  const parts: string[] = [];
  for (const [i, segment] of key.segments.entries()) {
    if (segment === undefined) continue;
    const evaluated = evaluateSegment(key.name, i, segment, input);
    if (!evaluated.omit) {
      parts.push(
        `${segment.prefix ?? ''}${evaluated.value}${segment.suffix ?? ''}`,
      );
    }
  }
  return parts.join(key.join);
}

export function verifyDerivedKeyFixtures(contract: DerivedKeyContract): void {
  validateDerivedKeyContract(contract);
  for (const key of contract.derived_keys) {
    for (const fixture of key.fixtures ?? []) {
      const got = evaluateDerivedKeyDefinition(key, fixture.input);
      if (got !== fixture.expect) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${key.name} fixture ${fixture.name}: expected ${JSON.stringify(
            fixture.expect,
          )}, got ${JSON.stringify(got)}`,
        );
      }
    }
  }
}

export function validateDerivedKeyContract(
  value: unknown,
): asserts value is DerivedKeyContract {
  if (!isPlainObject(value)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Key contract document must be an object',
    );
  }
  if (value.tabletheory_model_contract_version !== '0.1') {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Unsupported tabletheory_model_contract_version: ${String(
        value.tabletheory_model_contract_version ?? '',
      )}`,
    );
  }
  if (!Array.isArray(value.derived_keys) || value.derived_keys.length === 0) {
    throw new TheorydbError(
      'ErrInvalidModel',
      'Key contract must include derived_keys[]',
    );
  }

  const names = new Set<string>();
  for (const key of value.derived_keys) {
    validateDerivedKeyDefinition(key);
    if (names.has(key.name)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Duplicate derived key: ${key.name}`,
      );
    }
    names.add(key.name);
  }

  if (value.models !== undefined) {
    if (!Array.isArray(value.models)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        'Key contract models must be an array',
      );
    }
    for (const model of value.models) {
      if (
        !isPlainObject(model) ||
        typeof model.name !== 'string' ||
        model.name === ''
      ) {
        throw new TheorydbError(
          'ErrInvalidModel',
          'Model contract missing name',
        );
      }
      if (model.derived_keys !== undefined) {
        if (!Array.isArray(model.derived_keys)) {
          throw new TheorydbError(
            'ErrInvalidModel',
            `Model ${model.name} derived_keys must be an array`,
          );
        }
        for (const keyName of model.derived_keys) {
          if (typeof keyName !== 'string' || !names.has(keyName)) {
            throw new TheorydbError(
              'ErrInvalidModel',
              `Model ${model.name} references unknown derived key: ${String(keyName)}`,
            );
          }
        }
      }
    }
  }
}

export function validateDerivedKeyDefinition(
  value: unknown,
): asserts value is DerivedKeyDefinition {
  if (
    !isPlainObject(value) ||
    typeof value.name !== 'string' ||
    value.name === ''
  ) {
    throw new TheorydbError('ErrInvalidModel', 'Derived key missing name');
  }
  if (typeof value.join !== 'string') {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Derived key ${value.name}: join must be a string`,
    );
  }
  if (!Array.isArray(value.segments) || value.segments.length === 0) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Derived key ${value.name}: missing segments[]`,
    );
  }

  const inputNames = new Set<string>();
  if (value.inputs !== undefined) {
    if (!Array.isArray(value.inputs)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Derived key ${value.name}: inputs must be an array`,
      );
    }
    for (const input of value.inputs) {
      if (
        !isPlainObject(input) ||
        typeof input.name !== 'string' ||
        input.name === ''
      ) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${value.name}: input missing name`,
        );
      }
      if (inputNames.has(input.name)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${value.name}: duplicate input ${input.name}`,
        );
      }
      inputNames.add(input.name);
    }
  }

  for (let i = 0; i < value.segments.length; i++) {
    validateSegment(value.name, i, value.segments[i], inputNames);
  }

  if (value.fixtures !== undefined) {
    if (!Array.isArray(value.fixtures)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Derived key ${value.name}: fixtures must be an array`,
      );
    }
    const fixtureNames = new Set<string>();
    for (const fixture of value.fixtures) {
      if (
        !isPlainObject(fixture) ||
        typeof fixture.name !== 'string' ||
        fixture.name === ''
      ) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${value.name}: fixture missing name`,
        );
      }
      if (fixtureNames.has(fixture.name)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${value.name}: duplicate fixture ${fixture.name}`,
        );
      }
      fixtureNames.add(fixture.name);
      if (!isPlainObject(fixture.input)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${value.name} fixture ${fixture.name}: missing input`,
        );
      }
      if (typeof fixture.expect !== 'string') {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${value.name} fixture ${fixture.name}: expect must be a string`,
        );
      }
    }
  }
}

function validateSegment(
  keyName: string,
  index: number,
  value: unknown,
  inputNames: ReadonlySet<string>,
): asserts value is DerivedKeySegment {
  const label =
    isPlainObject(value) && typeof value.name === 'string' && value.name !== ''
      ? value.name
      : `#${index + 1}`;
  if (!isPlainObject(value)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Derived key ${keyName} segment ${label}: segment must be an object`,
    );
  }

  const sourceCount =
    (typeof value.literal === 'string' ? 1 : 0) +
    (isPlainObject(value.value) &&
    typeof value.value.input === 'string' &&
    value.value.input !== ''
      ? 1
      : 0) +
    (isPlainObject(value.value) && typeof value.value.literal === 'string'
      ? 1
      : 0);
  if (sourceCount !== 1) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Derived key ${keyName} segment ${label}: exactly one value source is required`,
    );
  }

  const inputName =
    isPlainObject(value.value) && typeof value.value.input === 'string'
      ? value.value.input
      : '';
  if (inputName !== '' && inputNames.size > 0 && !inputNames.has(inputName)) {
    throw new TheorydbError(
      'ErrInvalidModel',
      `Derived key ${keyName} segment ${label}: unknown input ${inputName}`,
    );
  }

  if (value.transforms !== undefined) {
    if (!Array.isArray(value.transforms)) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Derived key ${keyName} segment ${label}: transforms must be an array`,
      );
    }
    for (const transform of value.transforms) {
      if (transform !== 'trim' && transform !== 'wildcard_empty') {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${keyName} segment ${label}: unsupported transform ${String(transform)}`,
        );
      }
    }
  }
}

function evaluateSegment(
  keyName: string,
  index: number,
  segment: DerivedKeySegment,
  input: Record<string, KeyContractInputValue>,
): { value: string; omit: boolean } {
  const label =
    segment.name && segment.name !== '' ? segment.name : `#${index + 1}`;
  const resolved = resolveSegmentValue(segment, input, keyName, label);
  if (!resolved.present && segment.optional && segment.default === undefined) {
    return { value: '', omit: true };
  }

  let value = resolved.value;
  for (const transform of segment.transforms ?? []) {
    switch (transform) {
      case 'trim':
        value = value.trim();
        break;
      case 'wildcard_empty':
        if (value === '') value = '*';
        break;
      default:
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key ${keyName} segment ${label}: unsupported transform ${String(transform)}`,
        );
    }
  }

  if (value === '' && segment.default !== undefined) {
    value = segment.default;
  }

  if (shouldOmitSegment(value, segment)) {
    return { value: '', omit: true };
  }
  if (value === '' && segment.optional && segment.default === undefined) {
    return { value: '', omit: true };
  }
  return { value, omit: false };
}

function resolveSegmentValue(
  segment: DerivedKeySegment,
  input: Record<string, KeyContractInputValue>,
  keyName: string,
  label: string,
): { value: string; present: boolean } {
  if (segment.literal !== undefined)
    return { value: segment.literal, present: true };
  if (segment.value?.literal !== undefined) {
    return { value: segment.value.literal, present: true };
  }
  const inputName = segment.value?.input;
  if (inputName) {
    if (!Object.prototype.hasOwnProperty.call(input, inputName)) {
      if (segment.default !== undefined || segment.optional) {
        return { value: '', present: false };
      }
      throw new TheorydbError(
        'ErrInvalidModel',
        `Derived key ${keyName} segment ${label}: missing required input ${JSON.stringify(inputName)}`,
      );
    }
    return {
      value: scalarToString(input[inputName], inputName),
      present: true,
    };
  }
  throw new TheorydbError(
    'ErrInvalidModel',
    `Derived key ${keyName} segment ${label}: missing value source`,
  );
}

function shouldOmitSegment(value: string, segment: DerivedKeySegment): boolean {
  if (segment.omit_when?.empty && value === '') return true;
  if (
    segment.omit_when?.default &&
    segment.default !== undefined &&
    value === segment.default
  ) {
    return true;
  }
  for (const candidate of segment.omit_when?.values ?? []) {
    if (value === candidate) return true;
  }
  return false;
}

function scalarToString(value: unknown, inputName: string): string {
  if (value === null || value === undefined) return '';
  switch (typeof value) {
    case 'string':
      return value;
    case 'number':
      if (!Number.isFinite(value)) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Derived key input ${inputName} must be a finite number`,
        );
      }
      return String(value);
    case 'boolean':
      return value ? 'true' : 'false';
    default:
      throw new TheorydbError(
        'ErrInvalidModel',
        `Derived key input ${inputName} must be a scalar`,
      );
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value))
    return false;
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}
