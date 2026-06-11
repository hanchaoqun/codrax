package inv.rpc;

import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

/** v3-current server implementation over the vendored stub surface. */
public final class InventoryService implements InventoryGrpc.InventoryImplBase {
    private final Map<String, Integer> stock = new HashMap<>();

    @Override
    public InventoryGrpc.ItemReply getItem(InventoryGrpc.ItemRequest request) {
        InventoryGrpc.ItemReply reply = new InventoryGrpc.ItemReply();
        reply.sku = request.sku;
        reply.available = stock.getOrDefault(request.sku, 0);
        reply.warehouseId = "wh-east-1";
        return reply;
    }

    @Override
    public InventoryGrpc.ListReply listItems(InventoryGrpc.ListRequest request) {
        InventoryGrpc.ListReply reply = new InventoryGrpc.ListReply();
        reply.items = new InventoryGrpc.ItemReply[0];
        reply.nextPageToken = "";
        return reply;
    }

    @Override
    public InventoryGrpc.ItemReply reserveItem(InventoryGrpc.ReserveRequest request) {
        int held = stock.getOrDefault(request.sku, 0);
        InventoryGrpc.ItemReply reply = new InventoryGrpc.ItemReply();
        reply.sku = request.sku;
        reply.available = Math.max(0, held - request.quantity);
        reply.reserved = held >= request.quantity;
        reply.warehouseId = "wh-east-1";
        stock.put(request.sku, reply.available);
        return reply;
    }

    @Override
    public InventoryGrpc.ItemReply releaseItem(InventoryGrpc.ReleaseRequest request) {
        InventoryGrpc.ItemReply reply = new InventoryGrpc.ItemReply();
        reply.sku = request.sku;
        reply.reserved = false;
        reply.warehouseId = "wh-east-1";
        if (request.reservationId == null || request.reservationId.isEmpty()) {
            reply.available = -1;
            return reply;
        }
        reply.available = stock.getOrDefault(request.sku, 0);
        return reply;
    }

    static String newReservationId() {
        return UUID.randomUUID().toString();
    }
}
