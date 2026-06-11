package app;

import app.handlers.EchoHandler;
import app.handlers.StatsHandler;
import app.handlers.UpperHandler;
import app.router.Router;

public final class Main {
    public static void main(String[] args) {
        AuditLog audit = new AuditLog();
        Router router = new Router(audit);
        router.register(new EchoHandler());
        router.register(new UpperHandler());
        router.register(new StatsHandler());

        System.out.println(router.dispatch("/stats", "hello world"));
        System.out.println(router.dispatch("/upper", "hello"));
        System.out.println(router.dispatch("/missing", ""));
    }
}
