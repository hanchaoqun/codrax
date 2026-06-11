package com.clinic.config;

import java.io.InputStream;
import java.util.Properties;

/**
 * Visit-capacity configuration with three precedence layers, lowest
 * to highest: the code default DEFAULT_MAX_VISITS, the packaged
 * application.properties (clinic.max-visits), and the CLINIC_MAX_VISITS
 * environment variable which always wins at runtime.
 */
public class ClinicConfig {
    public static final int DEFAULT_MAX_VISITS = 20;

    public int resolveMaxVisits() {
        int value = DEFAULT_MAX_VISITS;
        try (InputStream in = getClass().getResourceAsStream("/application.properties")) {
            if (in != null) {
                Properties props = new Properties();
                props.load(in);
                String fromFile = props.getProperty("clinic.max-visits");
                if (fromFile != null) {
                    value = Integer.parseInt(fromFile.trim());
                }
            }
        } catch (Exception ignored) {
            // fall through to the current best value
        }
        String fromEnv = System.getenv("CLINIC_MAX_VISITS");
        if (fromEnv != null && !fromEnv.isBlank()) {
            value = Integer.parseInt(fromEnv.trim());
        }
        return value;
    }
}
