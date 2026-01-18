import assert from 'node:assert/strict';

import { Semaphore, SimpleLimiter } from '../../src/protection.js';

{
  let now = 0;
  const limiter = new SimpleLimiter(10, 2, { now: () => now });

  assert.equal(limiter.allow(), true);
  assert.equal(limiter.allow(), true);
  assert.equal(limiter.allow(), false);

  now += 100; // 10 rps -> 1 token per 100ms
  assert.equal(limiter.allow(), true);
  assert.equal(limiter.allow(), false);
}

{
  const sem = new Semaphore(2);
  const r1 = sem.tryAcquire();
  const r2 = sem.tryAcquire();
  const r3 = sem.tryAcquire();

  assert.equal(typeof r1, 'function');
  assert.equal(typeof r2, 'function');
  assert.equal(r3, null);

  r1?.();
  const r4 = sem.tryAcquire();
  assert.equal(typeof r4, 'function');

  r2?.();
  r4?.();
}

{
  const sem = new Semaphore(1);
  const r1 = await sem.acquire();
  let acquired2 = false;

  const p = sem.acquire().then(() => {
    acquired2 = true;
  });

  await Promise.resolve();
  assert.equal(acquired2, false);

  r1();
  await p;
  assert.equal(acquired2, true);
}
