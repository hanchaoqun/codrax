"""Vendored stub surface generated from inventory.proto v2 — STALE.

v2 service surface: GetItem, ListItems, HoldItem. The v3 contract
renamed HoldItem to ReserveItem and added ReleaseItem; neither change
has been regenerated here. ItemReply predates the v3 warehouse_id
field.
"""

from dataclasses import dataclass, field


@dataclass
class ItemRequest:
    sku: str = ""


@dataclass
class HoldRequest:
    sku: str = ""
    quantity: int = 0


@dataclass
class ItemReply:
    sku: str = ""
    available: int = 0
    reserved: bool = False
    # NOTE: no warehouse_id — v2-era message shape.


@dataclass
class ListRequest:
    page_size: int = 0
    page_token: str = ""


@dataclass
class ListReply:
    items: list = field(default_factory=list)
    next_page_token: str = ""


class InventoryStub:
    """v2 client stub: three methods only."""

    def __init__(self, channel):
        self._channel = channel

    def GetItem(self, request: ItemRequest) -> ItemReply:
        return self._channel.unary("inventory.v2.Inventory/GetItem", request)

    def ListItems(self, request: ListRequest) -> ListReply:
        return self._channel.unary("inventory.v2.Inventory/ListItems", request)

    def HoldItem(self, request: HoldRequest) -> ItemReply:
        return self._channel.unary("inventory.v2.Inventory/HoldItem", request)
