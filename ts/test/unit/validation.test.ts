import assert from 'node:assert/strict';

import {
  MaxExpressionLength,
  MaxFieldNameLength,
  MaxNestedDepth,
  MaxOperatorLength,
  MaxValueStringLength,
  SecurityValidationError,
  validateExpression,
  validateFieldName,
  validateIndexName,
  validateOperator,
  validateTableName,
  validateValue,
} from '../../src/validation.js';

{
  const err = new SecurityValidationError('InvalidField', 'detail');
  assert.equal(err.message, 'security validation failed: InvalidField');
  assert.equal(err.type, 'InvalidField');
  assert.equal(err.detail, 'detail');
}

{
  const valid = [
    'UserID',
    'user_id',
    '_internal',
    'Name',
    'nested.field',
    'deeply.nested.field.name',
    'listField[0]',
  ];
  for (const name of valid) validateFieldName(name);
}

{
  assert.throws(
    () => validateFieldName(''),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidField');
      return true;
    },
  );
}

{
  const longName = 'a'.repeat(MaxFieldNameLength + 1);
  assert.throws(
    () => validateFieldName(longName),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidField');
      return true;
    },
  );
}

{
  const deepParts = Array.from({ length: MaxNestedDepth + 1 }, () => 'a');
  const deepName = deepParts.join('.');
  assert.throws(
    () => validateFieldName(deepName),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidField');
      return true;
    },
  );
}

{
  const bad = [
    "field'; DROP TABLE users; --",
    'field" ; DELETE FROM table; --',
    'field/*comment*/',
    'field UNION SELECT',
    "field<script>alert('xss')</script>",
  ];
  for (const name of bad) {
    assert.throws(
      () => validateFieldName(name),
      (err) => {
        assert.ok(err instanceof SecurityValidationError);
        assert.equal(err.type, 'InjectionAttempt');
        assert.ok(!err.message.includes('DROP TABLE'));
        assert.ok(!err.message.includes('<script'));
        return true;
      },
    );
  }
}

{
  assert.throws(
    () => validateFieldName('ok\u0000bad'),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidField');
      return true;
    },
  );
}

{
  for (const op of ['=', '!=', '<>', '<', '<=', '>', '>=', 'between', 'IN']) {
    validateOperator(op);
  }

  assert.throws(
    () => validateOperator(''),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidOperator');
      return true;
    },
  );

  assert.throws(
    () => validateOperator('X'.repeat(MaxOperatorLength + 1)),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidOperator');
      return true;
    },
  );

  assert.throws(
    () => validateOperator('INVALID_OP'),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidOperator');
      return true;
    },
  );
}

{
  validateValue(null);
  validateValue(undefined);
  validateValue('hello');
  validateValue({ a: 1, b: 'ok', c: true });
  validateValue([1, 2, 3]);

  assert.throws(
    () => validateValue('a'.repeat(MaxValueStringLength + 1)),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidValue');
      return true;
    },
  );

  assert.throws(
    () => validateValue("<script>alert('x')</script>"),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InjectionAttempt');
      assert.ok(!err.message.includes('<script'));
      return true;
    },
  );

  assert.throws(
    () => validateValue(Array.from({ length: 101 }, () => 1)),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidValue');
      return true;
    },
  );
}

{
  validateExpression('attribute_exists(#a) AND #b = :b');

  assert.throws(
    () => validateExpression('a'.repeat(MaxExpressionLength + 1)),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidExpression');
      return true;
    },
  );

  assert.throws(
    () => validateExpression('name = 1; DROP TABLE users; --'),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InjectionAttempt');
      assert.ok(!err.message.includes('DROP TABLE'));
      return true;
    },
  );
}

{
  validateTableName('users_table');
  validateTableName('users-table');
  validateTableName('users.table');

  assert.throws(
    () => validateTableName('bad name'),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidTableName');
      return true;
    },
  );

  assert.throws(
    () => validateTableName('users;drop'),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidTableName');
      return true;
    },
  );

  validateIndexName('');
  validateIndexName('gsi-email');

  assert.throws(
    () => validateIndexName('bad name'),
    (err) => {
      assert.ok(err instanceof SecurityValidationError);
      assert.equal(err.type, 'InvalidIndexName');
      return true;
    },
  );
}
