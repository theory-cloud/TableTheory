export class SimpleLimiter {
  private lastRefillMs: number;
  private tokens: number;
  private readonly maxTokens: number;
  private readonly refillIntervalMs: number;
  private readonly now: () => number;

  constructor(rps: number, burst: number, opts: { now?: () => number } = {}) {
    if (!Number.isFinite(rps) || rps <= 0) {
      throw new Error('rps must be a positive number');
    }
    if (!Number.isInteger(burst) || burst <= 0) {
      throw new Error('burst must be a positive integer');
    }

    this.now = opts.now ?? (() => Date.now());
    this.tokens = burst;
    this.maxTokens = burst;
    this.refillIntervalMs = 1000 / rps;
    this.lastRefillMs = this.now();
  }

  allow(): boolean {
    const now = this.now();
    const elapsed = now - this.lastRefillMs;
    const tokensToAdd = Math.floor(elapsed / this.refillIntervalMs);

    if (tokensToAdd > 0) {
      this.tokens = Math.min(this.maxTokens, this.tokens + tokensToAdd);
      this.lastRefillMs = now;
    }

    if (this.tokens > 0) {
      this.tokens -= 1;
      return true;
    }

    return false;
  }
}

type SemaphoreWaiter = {
  resolve: (release: () => void) => void;
  reject: (err: unknown) => void;
  signal: AbortSignal | undefined;
  onAbort: (() => void) | undefined;
};

export class Semaphore {
  private readonly capacity: number;
  private inUse = 0;
  private readonly waiters: SemaphoreWaiter[] = [];

  constructor(capacity: number) {
    if (!Number.isInteger(capacity) || capacity <= 0) {
      throw new Error('capacity must be a positive integer');
    }
    this.capacity = capacity;
  }

  tryAcquire(): (() => void) | null {
    if (this.inUse >= this.capacity) return null;
    this.inUse += 1;
    return () => this.release();
  }

  async acquire(opts: { signal?: AbortSignal } = {}): Promise<() => void> {
    const immediate = this.tryAcquire();
    if (immediate) return immediate;

    const { signal } = opts;
    if (signal?.aborted) throw abortError(signal);

    return await new Promise<() => void>((resolve, reject) => {
      const waiter: SemaphoreWaiter = {
        resolve,
        reject,
        signal,
        onAbort: undefined,
      };

      if (signal) {
        const onAbort = () => {
          const idx = this.waiters.indexOf(waiter);
          if (idx >= 0) this.waiters.splice(idx, 1);
          reject(abortError(signal));
        };
        waiter.onAbort = onAbort;
        signal.addEventListener('abort', onAbort, { once: true });
      }

      this.waiters.push(waiter);
    });
  }

  private release(): void {
    if (this.inUse <= 0) throw new Error('semaphore release underflow');

    const waiter = this.waiters.shift();
    if (!waiter) {
      this.inUse -= 1;
      return;
    }

    if (waiter.signal && waiter.onAbort) {
      waiter.signal.removeEventListener('abort', waiter.onAbort);
    }

    waiter.resolve(() => this.release());
  }
}

function abortError(signal: AbortSignal): Error {
  if (signal.reason instanceof Error) return signal.reason;
  return new Error('aborted');
}
