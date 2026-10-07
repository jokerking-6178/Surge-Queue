package com.surge.backend.model;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public class RegistrationRequest {

    @NotBlank(message = "name is required")
    @Size(max = 200)
    private String name;

    @NotBlank(message = "email is required")
    @Size(max = 320)
    private String email;

    @Size(max = 500)
    private String details;

    // --- getters / setters ---

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }

    public String getEmail() { return email; }
    public void setEmail(String email) { this.email = email; }

    public String getDetails() { return details; }
    public void setDetails(String details) { this.details = details; }
}
