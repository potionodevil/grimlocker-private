package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonIgnoreProperties(ignoreUnknown = true)
public class Workspace {

    @JsonProperty("id")         public String id;
    @JsonProperty("name")       public String name;
    @JsonProperty("is_default") public boolean isDefault;
    @JsonProperty("created_at") public long createdAt;

    @Override
    public String toString() {
        return "Workspace{id='" + id + "', name='" + name + "', isDefault=" + isDefault + "}";
    }
}
