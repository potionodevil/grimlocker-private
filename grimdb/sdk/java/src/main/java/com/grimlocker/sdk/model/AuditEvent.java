package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonIgnoreProperties(ignoreUnknown = true)
public class AuditEvent {

    @JsonProperty("timestamp")  public long timestamp;
    @JsonProperty("level")      public String level;
    @JsonProperty("module")     public String module;
    @JsonProperty("message")    public String message;
    @JsonProperty("subject_id") public String subjectId;

    @Override
    public String toString() {
        return "AuditEvent{level='" + level + "', module='" + module + "', message='" + message + "'}";
    }
}
