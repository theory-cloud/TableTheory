import js from '@eslint/js';
import globals from 'globals';
import tseslint from '@typescript-eslint/eslint-plugin';
import tsParser from '@typescript-eslint/parser';

const deferredTypeAwareRules = Object.fromEntries(
  [
    '@typescript-eslint/no-base-to-string',
    '@typescript-eslint/no-redundant-type-constituents',
    '@typescript-eslint/no-unnecessary-type-assertion',
    '@typescript-eslint/no-unsafe-argument',
    '@typescript-eslint/no-unsafe-assignment',
    '@typescript-eslint/no-unsafe-member-access',
    '@typescript-eslint/only-throw-error',
    '@typescript-eslint/require-await',
    '@typescript-eslint/restrict-template-expressions',
  ].map((rule) => [rule, 'off']),
);

/** @type {import("eslint").Linter.FlatConfig[]} */
export default [
  {
    ignores: ['dist/**', 'node_modules/**'],
  },
  js.configs.recommended,
  {
    files: ['**/*.ts'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: 'latest',
        projectService: {
          allowDefaultProject: ['examples/*.ts', 'examples/lambda/*.ts'],
        },
        sourceType: 'module',
        tsconfigRootDir: import.meta.dirname,
      },
      globals: {
        ...globals.es2022,
        ...globals.node,
      },
    },
    plugins: {
      '@typescript-eslint': tseslint,
    },
    rules: {
      ...tseslint.configs.recommended.rules,
      ...tseslint.configs['recommended-type-checked'].rules,
      // Deliberate typed-lint deferral: recommended-type-checked is active and
      // no-floating-promises stays enforced, but the exact legacy high-churn
      // rules below remain disabled until a dedicated typed-safety cleanup can
      // touch dynamic DynamoDB marshalling and testkit boundaries without
      // unrelated public behavior churn. Do not grow this allowlist casually.
      ...deferredTypeAwareRules,
      // TypeScript handles undefined checks for types/namespaces (e.g., `NodeJS`).
      'no-undef': 'off',
    },
  },
];
