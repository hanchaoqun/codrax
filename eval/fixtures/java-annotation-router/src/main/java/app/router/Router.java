package app.router;

import app.AuditLog;
import java.lang.reflect.Field;
import java.util.HashMap;
import java.util.Map;

/**
 * Annotation-driven router. register() reads each handler class's
 * {@code @Route} annotation for the path key and reflectively fills
 * {@code @Inject} fields with shared services before the instance
 * becomes reachable; dispatch() is a plain map lookup afterwards.
 */
public final class Router {
    private final Map<String, Handler> table = new HashMap<>();
    private final AuditLog auditLog;

    public Router(AuditLog auditLog) {
        this.auditLog = auditLog;
    }

    public void register(Handler handler) {
        Route route = handler.getClass().getAnnotation(Route.class);
        if (route == null) {
            throw new IllegalArgumentException(
                "handler " + handler.getClass().getName() + " lacks @Route");
        }
        injectFields(handler);
        Handler prior = table.putIfAbsent(route.path(), handler);
        if (prior != null) {
            throw new IllegalStateException("duplicate route: " + route.path());
        }
    }

    public String dispatch(String path, String body) {
        Handler handler = table.get(path);
        if (handler == null) {
            return "404 no handler for " + path;
        }
        return handler.handle(body);
    }

    private void injectFields(Handler handler) {
        for (Field field : handler.getClass().getDeclaredFields()) {
            if (!field.isAnnotationPresent(Inject.class)) {
                continue;
            }
            if (!field.getType().isAssignableFrom(AuditLog.class)) {
                throw new IllegalStateException(
                    "@Inject supports AuditLog only, got " + field.getType());
            }
            field.setAccessible(true);
            try {
                field.set(handler, auditLog);
            } catch (IllegalAccessException e) {
                throw new IllegalStateException("inject failed", e);
            }
        }
    }
}
