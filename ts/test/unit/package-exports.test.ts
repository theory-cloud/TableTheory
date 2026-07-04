import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const packageJson = JSON.parse(
  readFileSync(new URL('../../package.json', import.meta.url), 'utf8'),
) as {
  exports: Record<
    string,
    {
      import: { types: string; default: string };
      require: { types: string; default: string };
    }
  >;
};

void test('package exposes domain subpath exports', () => {
  assert.deepEqual(packageJson.exports['./facetheory'], {
    import: {
      types: './dist/facetheory-isr.d.ts',
      default: './dist/facetheory-isr.js',
    },
    require: {
      types: './dist/cjs/facetheory-isr.d.ts',
      default: './dist/cjs/facetheory-isr.js',
    },
  });
  assert.deepEqual(packageJson.exports['./release-state'], {
    import: {
      types: './dist/release-state.d.ts',
      default: './dist/release-state.js',
    },
    require: {
      types: './dist/cjs/release-state.d.ts',
      default: './dist/cjs/release-state.js',
    },
  });
  assert.deepEqual(packageJson.exports['./lease'], {
    import: { types: './dist/lease.d.ts', default: './dist/lease.js' },
    require: {
      types: './dist/cjs/lease.d.ts',
      default: './dist/cjs/lease.js',
    },
  });
});
