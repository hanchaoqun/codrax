import { API_ROUTES } from "./routes";

export class GatewayClient {
  constructor(private baseUrl: string) {}

  async search(query: string): Promise<unknown> {
    const r = API_ROUTES.search;
    const res = await fetch(this.baseUrl + r.path, {
      method: r.method,
      body: JSON.stringify({ query }),
    });
    return res.json();
  }
}
