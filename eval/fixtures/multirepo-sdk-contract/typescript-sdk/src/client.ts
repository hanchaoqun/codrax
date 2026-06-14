export interface TextSearchOptions {
  limit?: number;
  namespace?: string;
}

export class MemoryClient {
  constructor(private baseUrl: string) {}

  async textSearch(query: string, options: TextSearchOptions = {}): Promise<unknown> {
    const limit = options.limit ?? 10;
    const params = new URLSearchParams({
      q: query,
      limit: String(limit),
    });
    if (options.namespace) {
      params.set("namespace", options.namespace);
    }
    const res = await fetch(`${this.baseUrl}/v1/memories/search?${params.toString()}`, {
      method: "GET",
    });
    return res.json();
  }
}
