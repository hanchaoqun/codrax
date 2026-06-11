package com.acme.gateway;

public final class Route {
    public final String method;
    public final String path;

    public Route(String method, String path) {
        this.method = method;
        this.path = path;
    }
}
