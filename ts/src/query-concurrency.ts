import { TheorydbError } from './errors.js';

export async function mapConcurrent<T, R>(
  items: T[],
  concurrency: number,
  fn: (item: T) => Promise<R>,
  onError?: (error: unknown) => void,
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

  // Record the first failure observed and keep every worker running to
  // completion before rejecting. Lambda freezes the execution environment as
  // soon as the handler returns, so a worker left running past this call would
  // be frozen mid-flight and could resume against an invocation that is over.
  let firstError: unknown;
  let failed = false;

  const workers = Array.from({ length: limit }, async () => {
    let done = false;
    while (!done) {
      const idx = next;
      next += 1;
      if (idx >= items.length) {
        done = true;
        continue;
      }
      try {
        out[idx] = await fn(items[idx]!);
      } catch (error) {
        if (!failed) {
          failed = true;
          firstError = error;
        }
        onError?.(error);
        throw error;
      }
    }
  });

  // allSettled rather than all: every worker must settle before this returns,
  // so a rejection cannot abandon the workers that are still running.
  await Promise.allSettled(workers);

  if (failed) throw firstError;
  return out;
}
