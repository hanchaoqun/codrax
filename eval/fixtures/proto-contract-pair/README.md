# proto-contract-pair — IDL contract drift across sub-repos (multi-repo)

Blueprint shape: grpc multi-language SDKs where one side regenerates
stubs after a proto change and the other side lags.

Layout: three sub-repos (each git-inits at scratch setup):
  proto-defs/   — inventory.proto v3: THE source of truth
  java-server/  — stubs match proto v3 (current)
  py-client/    — stubs generated from proto v2 (STALE)

Design facts (cases assert exactly these):
- inventory.proto v3 declares FOUR rpcs: GetItem, ListItems,
  ReserveItem, ReleaseItem. The v2-era name "HoldItem" was RENAMED to
  ReserveItem in v3.
- java-server mirrors v3 exactly (four methods incl. reserveItem) and
  ItemReply carries the v3 field `warehouse_id` (tag 4).
- py-client is stale at v2: it still calls HoldItem (renamed away),
  lacks ReleaseItem entirely, and its ItemReply stub has no
  warehouse_id field.
- Therefore: the STALE side is py-client; java-server is current.
