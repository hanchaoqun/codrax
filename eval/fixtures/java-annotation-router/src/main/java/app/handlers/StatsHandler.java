package app.handlers;

import app.AuditLog;
import app.router.Handler;
import app.router.Inject;
import app.router.Route;

/**
 * Reports body length and records every request to the injected
 * audit trail. The auditLog field is filled by Router.injectFields
 * before this handler is registered — it is never constructed here.
 */
@Route(path = "/stats")
public final class StatsHandler implements Handler {
    @Inject
    private AuditLog auditLog;

    @Override
    public String handle(String body) {
        auditLog.record("/stats len=" + body.length());
        return "len=" + body.length() + " audited=" + auditLog.size();
    }
}
