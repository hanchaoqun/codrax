"""High-level client calls over the (stale) v2 stub surface."""

from .stubs import HoldRequest, InventoryStub, ItemReply, ItemRequest


class InventoryClient:
    def __init__(self, channel):
        self._stub = InventoryStub(channel)

    def availability(self, sku: str) -> int:
        reply: ItemReply = self._stub.GetItem(ItemRequest(sku=sku))
        return reply.available

    def hold(self, sku: str, quantity: int) -> bool:
        """Still calls the v2 HoldItem rpc — renamed server-side in v3."""
        reply = self._stub.HoldItem(HoldRequest(sku=sku, quantity=quantity))
        return reply.reserved
