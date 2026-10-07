package com.surge.backend.security;

import com.surge.backend.config.QueueSecurityProperties;
import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import javax.crypto.SecretKey;
import java.io.IOException;

/**
 * Validates the queue admit token (JWT) on every request.
 *
 * The frontend must first obtain an admit token from the queue engine and then
 * pass it as a Bearer token. If enforce-queue-token is false (demo "without queue"
 * mode), the filter passes through — this is how we show the backend collapsing
 * under direct load.
 */
@Component
public class QueueTokenFilter extends OncePerRequestFilter {

    private static final Logger log = LoggerFactory.getLogger(QueueTokenFilter.class);

    private final QueueSecurityProperties properties;
    private final SecretKey signingKey;

    public QueueTokenFilter(QueueSecurityProperties properties) {
        this.properties = properties;
        // Pad/derive a key — HS256 needs at least 256 bits.
        byte[] keyBytes = properties.getJwtSecret().getBytes();
        this.signingKey = Keys.hmacShaKeyFor(keyBytes);
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request,
                                    HttpServletResponse response,
                                    FilterChain filterChain) throws ServletException, IOException {
                                        if ("OPTIONS".equalsIgnoreCase(request.getMethod())) {
                                            filterChain.doFilter(request, response);
                                            return;
                                        }
        // Skip validation for public paths
        String path = request.getRequestURI();
        if (isPublicPath(path)) {
            filterChain.doFilter(request, response);
            return;
        }

        // If enforcement is disabled (demo mode), skip entirely
        if (!properties.isEnforceQueueToken()) {
            filterChain.doFilter(request, response);
            return;
        }

        String authHeader = request.getHeader("Authorization");
        if (authHeader == null || !authHeader.startsWith("Bearer ")) {
            respondError(response, HttpStatus.UNAUTHORIZED, "missing_admit_token",
                    "You must pass through the queue first. Obtain an admit token from /api/queue/admit.");
            return;
        }

        String token = authHeader.substring(7);
        try {
            Claims claims = Jwts.parser()
                    .verifyWith(signingKey)
                    .build()
                    .parseSignedClaims(token)
                    .getPayload();

            // Attach userId to request for downstream controllers
            request.setAttribute("userId", claims.getSubject());

        } catch (Exception e) {
            log.warn("invalid admit token: {}", e.getMessage());
            respondError(response, HttpStatus.UNAUTHORIZED, "invalid_admit_token",
                    "Your queue admit token is invalid or expired. Please re-enter the queue.");
            return;
        }

        filterChain.doFilter(request, response);
    }

    private boolean isPublicPath(String path) {
        return properties.getPublicPaths().stream().anyMatch(path::startsWith);
    }

    private void respondError(HttpServletResponse response, HttpStatus status,
                              String error, String message) throws IOException {
        response.setStatus(status.value());
        response.setContentType("application/json");
        response.getWriter().write(
                "{\"error\":\"" + error + "\",\"message\":\"" + message + "\"}"
        );
    }
}
