import assert from 'node:assert/strict';
import { test } from 'node:test';

const deferredTypeAwareRules = [
  '@typescript-eslint/no-base-to-string',
  '@typescript-eslint/no-redundant-type-constituents',
  '@typescript-eslint/no-unnecessary-type-assertion',
  '@typescript-eslint/no-unsafe-argument',
  '@typescript-eslint/no-unsafe-assignment',
  '@typescript-eslint/no-unsafe-member-access',
  '@typescript-eslint/only-throw-error',
  '@typescript-eslint/require-await',
  '@typescript-eslint/restrict-template-expressions',
];

void test('typed ESLint config keeps promise safety enforced with bounded legacy deferrals', async () => {
  const eslintConfig = (await import(
    new URL('../../eslint.config.js', import.meta.url).href
  )) as {
    default: Array<{
      files?: string[];
      rules?: Record<string, unknown>;
    }>;
  };

  const tsConfig = eslintConfig.default.find((config) =>
    config.files?.includes('**/*.ts'),
  );
  assert.ok(tsConfig?.rules);

  assert.equal(
    tsConfig.rules['@typescript-eslint/no-floating-promises'],
    'error',
  );

  for (const rule of deferredTypeAwareRules) {
    assert.equal(tsConfig.rules[rule], 'off');
  }

  const disabledTypedRules = Object.entries(tsConfig.rules)
    .filter(
      ([rule, setting]) =>
        rule.startsWith('@typescript-eslint/') && setting === 'off',
    )
    .map(([rule]) => rule)
    .sort();

  assert.deepEqual(disabledTypedRules, [...deferredTypeAwareRules].sort());
});
