package com.acme.gateway;

/**
 * Central route table for the gateway. The TypeScript SDK
 * (client-ts/src/routes.ts) mirrors these constants; drift between the
 * two files is a release blocker checked manually today.
 */
public final class Routes {
    public static final Route SEARCH = new Route("POST", "/v1/search");
    public static final Route GET_DOC = new Route("GET", "/v1/docs/{id}");
    public static final Route PUT_DOC = new Route("PUT", "/v1/docs/{id}");
    public static final Route DELETE_DOC = new Route("DELETE", "/v1/docs/{id}");
    public static final Route ADMIN_REINDEX = new Route("POST", "/v1/admin/reindex");

    private Routes() {}
}
