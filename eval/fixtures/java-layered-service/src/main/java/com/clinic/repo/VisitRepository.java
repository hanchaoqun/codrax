package com.clinic.repo;

import java.util.ArrayList;
import java.util.List;

/** Persistence layer; every mutation lands in the audit log. */
public class VisitRepository {
    private final List<String> rows = new ArrayList<>();
    private final AuditLog audit = new AuditLog();

    public int countOpenVisits(long petId) {
        int n = 0;
        for (String row : rows) {
            if (row.startsWith(petId + ":")) {
                n++;
            }
        }
        return n;
    }

    public long insert(long petId, String reason) {
        rows.add(petId + ":" + reason);
        audit.record("visit.insert", petId);
        return rows.size();
    }
}
