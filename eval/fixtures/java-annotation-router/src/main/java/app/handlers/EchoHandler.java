package app.handlers;

import app.router.Handler;
import app.router.Route;

/** Returns the body unchanged. */
@Route(path = "/echo")
public final class EchoHandler implements Handler {
    @Override
    public String handle(String body) {
        return body;
    }
}
