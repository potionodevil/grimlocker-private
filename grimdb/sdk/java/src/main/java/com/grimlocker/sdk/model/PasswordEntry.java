package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.grimlocker.sdk.Entry;
import java.util.Map;

@JsonIgnoreProperties(ignoreUnknown = true)
public class PasswordEntry {

    @JsonProperty("id")       public String id;
    @JsonProperty("title")    public String title;
    @JsonProperty("username") public String username;
    @JsonProperty("password") public String password;
    @JsonProperty("url")      public String url;
    @JsonProperty("notes")    public String notes;

    public Map<String, String> toFields() {
        return Map.of("username", username == null ? "" : username,
                      "password", password == null ? "" : password,
                      "url",      url == null ? "" : url,
                      "notes",    notes == null ? "" : notes);
    }

    public static PasswordEntry fromEntry(Entry e) {
        PasswordEntry p = new PasswordEntry();
        if (e == null) return p;
        p.id       = e.id;
        p.title    = e.title;
        p.username = e.field("username");
        p.password = e.field("password");
        p.url      = e.field("url");
        p.notes    = e.field("notes");
        return p;
    }

    @Override
    public String toString() {
        return "PasswordEntry{id='" + id + "', title='" + title + "', username='" + username + "'}";
    }
}
