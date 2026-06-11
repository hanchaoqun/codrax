package app.handlers;

import app.router.Handler;
import app.router.Route;

import java.util.Locale;

/** Upper-cases the body. */
@Route(path = "/upper")
public final class UpperHandler implements Handler {
    @Override
    public String handle(String body) {
        return body.toUpperCase(Locale.ROOT);
    }
}
