package inv.rpc;

/**
 * Hand-vendored stub surface generated from inventory.proto v3.
 * Method set mirrors the proto service exactly: getItem, listItems,
 * reserveItem (v3 rename of holdItem), releaseItem (v3 addition).
 */
public final class InventoryGrpc {

    public interface InventoryImplBase {
        ItemReply getItem(ItemRequest request);

        ListReply listItems(ListRequest request);

        ItemReply reserveItem(ReserveRequest request);

        ItemReply releaseItem(ReleaseRequest request);
    }

    public static final class ItemRequest {
        public String sku;
    }

    public static final class ListRequest {
        public int pageSize;
        public String pageToken;
    }

    public static final class ReserveRequest {
        public String sku;
        public int quantity;
    }

    public static final class ReleaseRequest {
        public String sku;
        public String reservationId;
    }

    public static final class ItemReply {
        public String sku;
        public int available;
        public boolean reserved;
        public String warehouseId; // v3 field (proto tag 4)
    }

    public static final class ListReply {
        public ItemReply[] items;
        public String nextPageToken;
    }

    private InventoryGrpc() {}
}
