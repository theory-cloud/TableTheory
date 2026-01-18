import assert from 'node:assert/strict';

import { chunk, sleep } from '../../src/batch.js';

assert.deepEqual(chunk([1, 2, 3, 4, 5], 2), [[1, 2], [3, 4], [5]]);

assert.throws(() => chunk([1], 0));

await sleep(1);
