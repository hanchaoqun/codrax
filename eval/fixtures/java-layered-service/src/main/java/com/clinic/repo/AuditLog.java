package com.clinic.repo;

/** Append-only audit trail; terminal hop of every write path. */
public class AuditLog {
    public void record(String action, long subjectId) {
        System.out.println("[audit] " + action + " subject=" + subjectId);
    }
}
