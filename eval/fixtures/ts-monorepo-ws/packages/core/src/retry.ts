// Retry policies for transport sends. The interface is deliberately
// tiny so policies stay swappable per-call.
export interface RetryPolicy {
  /** Maximum attempts including the first send. */
  maxAttempts(): number;
  /** Delay in milliseconds before attempt `n` (1-based retry index). */
  nextDelay(attempt: number): number;
}

/** Doubles the delay every retry, capped at `capMs`. */
export class ExponentialBackoff implements RetryPolicy {
  constructor(
    private readonly baseMs: number = 100,
    private readonly capMs: number = 5_000,
    private readonly attempts: number = 4,
  ) {}

  maxAttempts(): number {
    return this.attempts;
  }

  nextDelay(attempt: number): number {
    const raw = this.baseMs * 2 ** (attempt - 1);
    return Math.min(raw, this.capMs);
  }
}

/** Same delay for every retry — used by batch jobs that prefer
 * predictable pacing over fast recovery. */
export class FixedDelay implements RetryPolicy {
  constructor(
    private readonly delayMs: number = 250,
    private readonly attempts: number = 3,
  ) {}

  maxAttempts(): number {
    return this.attempts;
  }

  nextDelay(_attempt: number): number {
    return this.delayMs;
  }
}

/** Plain helper — adds bounded random jitter to a delay. NOT a
 * RetryPolicy implementation; policies call it when composing. */
export class JitterHelper {
  constructor(private readonly spreadMs: number = 50) {}

  apply(delayMs: number): number {
    return delayMs + Math.floor(Math.random() * this.spreadMs);
  }
}
