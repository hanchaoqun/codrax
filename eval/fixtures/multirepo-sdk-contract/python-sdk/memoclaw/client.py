from urllib.parse import urlencode


class MemoryClient:
    def __init__(self, base_url, transport):
        self.base_url = base_url.rstrip("/")
        self.transport = transport

    def _request(self, method, url, json=None):
        return self.transport.request(method, url, json=json)

    def text_search(self, query, limit=10, namespace=None):
        params = {"q": query, "limit": limit}
        if namespace:
            params["namespace"] = namespace
        url = f"{self.base_url}/v1/memories/search?{urlencode(params)}"
        return self._request("GET", url)


class AsyncMemoryClient:
    def __init__(self, base_url, transport):
        self.base_url = base_url.rstrip("/")
        self.transport = transport

    async def _request(self, method, url, json=None):
        return await self.transport.request(method, url, json=json)

    async def text_search(self, query, limit=10, namespace=None):
        params = {"q": query, "limit": limit}
        if namespace:
            params["namespace"] = namespace
        url = f"{self.base_url}/v1/memories/search?{urlencode(params)}"
        return await self._request("GET", url)
