package com.clinic.service;

import com.clinic.config.ClinicConfig;
import com.clinic.repo.VisitRepository;

/** Business rules: capacity check, then persistence + audit. */
public class VisitService {
    private final VisitRepository repository;
    private final ClinicConfig config;

    public VisitService(VisitRepository repository, ClinicConfig config) {
        this.repository = repository;
        this.config = config;
    }

    public long schedule(long petId, String reason) {
        int max = config.resolveMaxVisits();
        if (repository.countOpenVisits(petId) >= max) {
            throw new IllegalStateException("visit capacity reached: " + max);
        }
        return repository.insert(petId, reason);
    }
}
