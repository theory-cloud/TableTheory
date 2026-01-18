import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';

import { decodeCursor, encodeCursor } from '../../src/cursor.js';

const repoRoot = path.resolve(process.cwd(), '..');
const cursorPath = path.join(
  repoRoot,
  'contract-tests/golden/cursor/cursor_v0.1_basic.cursor',
);
const jsonPath = path.join(
  repoRoot,
  'contract-tests/golden/cursor/cursor_v0.1_basic.json',
);

const expectedEncoded = (await fs.readFile(cursorPath, 'utf8')).trim();
const expectedJson = JSON.parse(await fs.readFile(jsonPath, 'utf8')) as {
  lastKey: Record<string, unknown>;
  index?: string;
  sort?: 'ASC' | 'DESC';
};

const decoded = decodeCursor(expectedEncoded);
assert.deepEqual(decoded.lastKey, expectedJson.lastKey);
assert.equal(decoded.index, expectedJson.index);
assert.equal(decoded.sort, expectedJson.sort);

const reencoded = encodeCursor(decoded);
assert.equal(reencoded, expectedEncoded);
