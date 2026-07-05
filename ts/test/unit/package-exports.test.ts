import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const require = createRequire(import.meta.url);
const packageRoot = fileURLToPath(new URL('../../', import.meta.url));
const packageJson = JSON.parse(
  readFileSync(new URL('../../package.json', import.meta.url), 'utf8'),
) as {
  name: string;
  exports: Record<
    string,
    {
      import: { types: string; default: string };
      require: { types: string; default: string };
    }
  >;
};
const rootPackageSpecifier: string = packageJson.name;
let packageArtifactsBuilt = false;

function ensurePackageArtifactsBuilt(): void {
  if (packageArtifactsBuilt) {
    return;
  }
  const npmBinary = process.platform === 'win32' ? 'npm.cmd' : 'npm';
  execFileSync(npmBinary, ['run', 'build'], {
    cwd: packageRoot,
    stdio: 'inherit',
  });
  packageArtifactsBuilt = true;
}

const domainSubpaths = [
  {
    subpath: './facetheory',
    specifier: '@theory-cloud/tabletheory-ts/facetheory',
    runtimeSymbols: [
      'defineFaceTheoryCacheMetadataModel',
      'FaceTheoryIsrMetaStore',
      'createFaceTheoryIsrMetaStore',
    ],
    expected: {
      import: {
        types: './dist/facetheory-isr.d.ts',
        default: './dist/facetheory-isr.js',
      },
      require: {
        types: './dist/cjs/facetheory-isr.d.ts',
        default: './dist/cjs/facetheory-isr.js',
      },
    },
  },
  {
    subpath: './release-state',
    specifier: '@theory-cloud/tabletheory-ts/release-state',
    runtimeSymbols: [
      'transitionReleaseState',
      'validateDeployAuthorityMetadata',
    ],
    expected: {
      import: {
        types: './dist/release-state.d.ts',
        default: './dist/release-state.js',
      },
      require: {
        types: './dist/cjs/release-state.d.ts',
        default: './dist/cjs/release-state.js',
      },
    },
  },
  {
    subpath: './lease',
    specifier: '@theory-cloud/tabletheory-ts/lease',
    runtimeSymbols: ['LeaseManager'],
    expected: {
      import: { types: './dist/lease.d.ts', default: './dist/lease.js' },
      require: {
        types: './dist/cjs/lease.d.ts',
        default: './dist/cjs/lease.js',
      },
    },
  },
] as const;

const rootExcludedDomainSymbols = [
  'defineFaceTheoryCacheMetadataModel',
  'FaceTheoryIsrMetaStore',
  'createFaceTheoryIsrMetaStore',
  'transitionReleaseState',
  'validateDeployAuthorityMetadata',
  'LeaseManager',
] as const;

void test('package exposes domain subpath exports', () => {
  for (const entry of domainSubpaths) {
    assert.deepEqual(packageJson.exports[entry.subpath], entry.expected);
  }
});

void test('domain subpath exports resolve and load ESM and CommonJS artifacts', async () => {
  ensurePackageArtifactsBuilt();

  for (const entry of domainSubpaths) {
    const cjsResolved = require.resolve(entry.specifier);
    assert.equal(
      relative(packageRoot, cjsResolved).split(sep).join('/'),
      entry.expected.require.default.replace(/^\.\//, ''),
    );

    const cjsModule = require(entry.specifier) as Record<string, unknown>;
    const esmModule = (await import(entry.specifier)) as Record<
      string,
      unknown
    >;

    for (const symbol of entry.runtimeSymbols) {
      assert.equal(typeof cjsModule[symbol], 'function');
      assert.equal(typeof esmModule[symbol], 'function');
    }
  }
});

void test('root package excludes domain helper exports', async () => {
  ensurePackageArtifactsBuilt();

  const cjsModule = require(rootPackageSpecifier) as Record<string, unknown>;
  const esmModule = (await import(rootPackageSpecifier)) as Record<
    string,
    unknown
  >;

  assert.equal(typeof cjsModule.TheorydbClient, 'function');
  assert.equal(typeof esmModule.TheorydbClient, 'function');

  for (const symbol of rootExcludedDomainSymbols) {
    assert.equal(
      Object.hasOwn(cjsModule, symbol),
      false,
      `${symbol} leaked from CJS root`,
    );
    assert.equal(
      Object.hasOwn(esmModule, symbol),
      false,
      `${symbol} leaked from ESM root`,
    );
  }
});
