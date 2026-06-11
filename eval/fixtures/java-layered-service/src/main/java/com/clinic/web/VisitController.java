package com.clinic.web;

import com.clinic.service.VisitService;

/** HTTP boundary: validates the request shape, then delegates. */
public class VisitController {
    private final VisitService service;

    public VisitController(VisitService service) {
        this.service = service;
    }

    /** POST /visits entry point. */
    public long create(long petId, String reason) {
        if (reason == null || reason.isBlank()) {
            throw new IllegalArgumentException("reason is required");
        }
        return service.schedule(petId, reason);
    }
}
