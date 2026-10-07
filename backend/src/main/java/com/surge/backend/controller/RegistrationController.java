package com.surge.backend.controller;

import com.surge.backend.model.RegistrationRequest;
import com.surge.backend.service.RegistrationService;
import io.micrometer.core.instrument.Timer;
import io.micrometer.core.instrument.MeterRegistry;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.validation.Valid;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.Map;

@RestController
@RequestMapping("/api")
public class RegistrationController {

    private static final Logger log = LoggerFactory.getLogger(RegistrationController.class);

    private final RegistrationService registrationService;
    private final Timer registrationTimer;

    public RegistrationController(RegistrationService registrationService, MeterRegistry meterRegistry) {
        this.registrationService = registrationService;
        this.registrationTimer = Timer.builder("surge_backend_registration_duration")
                .description("Time taken to process a registration")
                .register(meterRegistry);
    }

    /**
     * POST /api/register
     * Protected by QueueTokenFilter — requires a valid admit token.
     */
    @PostMapping("/register")
    public ResponseEntity<?> register(@Valid @RequestBody RegistrationRequest req,
                                       HttpServletRequest request) {
        String userId = (String) request.getAttribute("userId");
        if (userId == null) {
            // Should not happen if filter enforces, but guard anyway
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED)
                    .body(Map.of("error", "no_user_id", "message", "Missing user identity"));
        }

        return registrationTimer.record(() -> {
            var result = registrationService.register(userId, req);
            return ResponseEntity.ok(Map.of(
                    "status", "success",
                    "registration_id", result.registrationId(),
                    "total_registrations", result.totalRegistrations()
            ));
        });
    }

    /**
     * GET /api/stats — returns current registration count.
     * Useful for the demo dashboard.
     */
    @GetMapping("/stats")
    public ResponseEntity<?> stats() {
        return ResponseEntity.ok(Map.of(
                "total_registrations", registrationService.getTotalRegistrations()
        ));
    }

    /**
     * GET /api/health — simple endpoint to prove the backend is alive.
     */
    @GetMapping("/health")
    public ResponseEntity<?> health() {
        return ResponseEntity.ok(Map.of("status", "ok", "service", "surge-backend"));
    }
}
