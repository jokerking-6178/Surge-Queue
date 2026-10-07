package com.surge.backend.config;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.context.annotation.Configuration;

import java.util.Arrays;
import java.util.List;

/**
 * Configuration properties for queue-token validation.
 */
@Configuration
@ConfigurationProperties(prefix = "surge.queue")
public class QueueSecurityProperties {

    @Value("${surge.queue.jwt-secret}")
    private String jwtSecret;

    @Value("${surge.queue.enforce-queue-token:true}")
    private boolean enforceQueueToken;

    @Value("${surge.queue.public-paths:/actuator,/healthz,/readyz}")
    private String publicPathsRaw;

    public String getJwtSecret() {
        return jwtSecret;
    }

    public boolean isEnforceQueueToken() {
        return enforceQueueToken;
    }

    public List<String> getPublicPaths() {
        return Arrays.asList(publicPathsRaw.split(","));
    }
}
