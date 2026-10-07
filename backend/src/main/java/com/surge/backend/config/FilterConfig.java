package com.surge.backend.config;

import com.surge.backend.security.QueueTokenFilter;
import org.springframework.boot.web.servlet.FilterRegistrationBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Registers the QueueTokenFilter in the servlet chain, running before controllers.
 */
@Configuration
public class FilterConfig {

    @Bean
    public FilterRegistrationBean<QueueTokenFilter> queueTokenFilterRegistration(
            QueueTokenFilter queueTokenFilter) {
        FilterRegistrationBean<QueueTokenFilter> registration = new FilterRegistrationBean<>();
        registration.setFilter(queueTokenFilter);
        registration.addUrlPatterns("/api/*");
        registration.setOrder(1);
        return registration;
    }
}
