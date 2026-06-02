package com.grimlocker.sdk;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.Collections;
import java.util.Map;

/** A single vault entry returned by the Grimlocker daemon. */
@JsonIgnoreProperties(ignoreUnknown = true)
public class Entry {

    @JsonProperty("id")
    public String id;

    @JsonProperty("category")
    public String category;

    @JsonProperty("title")
    public String title;

    @JsonProperty("fields")
    public Map<String, String> fields = Collections.emptyMap();

    @JsonProperty("created_at")
    public long createdAt;

    @JsonProperty("updated_at")
    public long updatedAt;

    public String field(String key) {
        return fields.getOrDefault(key, "");
    }

    @Override
    public String toString() {
        return "Entry{id='" + id + "', category='" + category + "', title='" + title + "'}";
    }
}
