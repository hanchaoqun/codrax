import { HttpTransport, FixedDelay, TransportResponse } from "@app/core";

export interface UserRecord {
  id: string;
  name: string;
  active: boolean;
}

/** Typed API facade over the core transport. User lookups use the
 * FixedDelay policy: profile reads are latency-tolerant but must not
 * hammer a degraded backend with exponential bursts. */
export class ApiClient {
  private readonly transport: HttpTransport;

  constructor(baseUrl: string) {
    this.transport = new HttpTransport(baseUrl, new FixedDelay(200, 3));
  }

  async fetchUser(id: string): Promise<UserRecord> {
    const resp = await this.transport.send(
      "/v1/users/lookup",
      JSON.stringify({ id }),
    );
    return parseUser(resp);
  }
}

function parseUser(resp: TransportResponse): UserRecord {
  if (resp.status !== 200) {
    throw new Error(`user lookup failed: status=${resp.status}`);
  }
  const raw = JSON.parse(resp.body) as Partial<UserRecord>;
  if (typeof raw.id !== "string" || typeof raw.name !== "string") {
    throw new Error("user payload missing id/name");
  }
  return { id: raw.id, name: raw.name, active: raw.active === true };
}
