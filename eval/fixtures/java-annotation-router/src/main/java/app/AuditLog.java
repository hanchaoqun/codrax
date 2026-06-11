package app;

import java.util.ArrayList;
import java.util.List;

/** Shared append-only audit trail, injected into handlers via @Inject. */
public final class AuditLog {
    private final List<String> entries = new ArrayList<>();

    public synchronized void record(String event) {
        entries.add(event);
    }

    public synchronized int size() {
        return entries.size();
    }
}
