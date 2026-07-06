import { TheorydbError } from './errors.js';

export async function mapConcurrent<T, R>(
  items: T[],
  concurrency: number,
  fn: (item: T) => Promise<R>,
): Promise<R[]> {
  if (!Number.isFinite(concurrency) || concurrency <= 0) {
    throw new TheorydbError(
      'ErrInvalidOperator',
      'concurrency must be a positive number',
    );
  }
  if (items.length === 0) return [];

  const limit = Math.min(items.length, Math.floor(concurrency));
  const out: R[] = new Array<R>(items.length);
  let next = 0;

  const workers = Array.from({ length: limit }, async () => {
    let done = false;
    while (!done) {
      const idx = next;
      next += 1;
      if (idx >= items.length) {
        done = true;
        continue;
      }
      out[idx] = await fn(items[idx]!);
    }
  });
  await Promise.all(workers);
  return out;
}
