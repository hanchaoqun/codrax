import { RetryPolicy, ExponentialBackoff } from "./retry";

export interface TransportResponse {
  status: number;
  body: string;
}

export interface Transport {
  send(path: string, payload: string): Promise<TransportResponse>;
}

/** HTTP transport with policy-driven retries. Retries only on
 * status >= 500; 4xx returns immediately. */
export class HttpTransport implements Transport {
  constructor(
    private readonly baseUrl: string,
    private readonly retry: RetryPolicy = new ExponentialBackoff(),
  ) {}

  async send(path: string, payload: string): Promise<TransportResponse> {
    let lastError: TransportResponse = { status: 0, body: "" };
    for (let attempt = 1; attempt <= this.retry.maxAttempts(); attempt++) {
      const resp = await this.dispatchOnce(path, payload);
      if (resp.status < 500) {
        return resp;
      }
      lastError = resp;
      if (attempt < this.retry.maxAttempts()) {
        await sleep(this.retry.nextDelay(attempt));
      }
    }
    return lastError;
  }

  private async dispatchOnce(
    path: string,
    payload: string,
  ): Promise<TransportResponse> {
    const url = `${this.baseUrl}${path}`;
    const resp = await fetch(url, { method: "POST", body: payload });
    return { status: resp.status, body: await resp.text() };
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
