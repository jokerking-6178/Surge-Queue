package com.surge.backend.service;

import com.surge.backend.model.RegistrationRequest;
import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * In-memory registration service. In production this would write to a database,
 * but for the demo the in-memory store is enough to show the queue protecting
 * a resource that has limited throughput.
 */
@Service
public class RegistrationService {

    private static final Logger log = LoggerFactory.getLogger(RegistrationService.class);

    // Simulated "database"
    private final Map<String, RegistrationRecord> store = new ConcurrentHashMap<>();
    private final AtomicInteger totalRegistrations = new AtomicInteger(0);

    // Metrics
    private final Counter registrationsCounter;
    private final Counter rejectionsCounter;

    public RegistrationService(MeterRegistry meterRegistry) {
        this.registrationsCounter = Counter.builder("surge_backend_registrations_total")
                .description("Total successful registrations")
                .register(meterRegistry);
        this.rejectionsCounter = Counter.builder("surge_backend_rejections_total")
                .description("Total registration rejections")
                .register(meterRegistry);
    }

    /**
     * Processes a registration. Simulates some work so the backend has real
     * per-request cost (e.g. DB write, email send).
     */
    public RegistrationResult register(String userId, RegistrationRequest req) {
        // Simulate processing latency (DB write + email enqueue)
        try {
            Thread.sleep(50); // 50ms per registration — simulates real backend work
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }

        String registrationId = UUID.randomUUID().toString();
        RegistrationRecord record = new RegistrationRecord(
                registrationId, userId, req.getName(), req.getEmail(), req.getDetails(),
                System.currentTimeMillis()
        );

        store.put(registrationId, record);
        int currentTotal = totalRegistrations.incrementAndGet();
        registrationsCounter.increment();

        log.info("registered userId={} name={} as regId={} (total={})",
                userId, req.getName(), registrationId, currentTotal);

        return new RegistrationResult(registrationId, "registered", currentTotal);
    }

    public int getTotalRegistrations() {
        return totalRegistrations.get();
    }

    // --- inner types ---

    public record RegistrationRecord(String id, String userId, String name,
                                      String email, String details, long timestamp) {}

    public record RegistrationResult(String registrationId, String status, int totalRegistrations) {}
}
