package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.grimlocker.sdk.Entry;
import java.util.Map;

@JsonIgnoreProperties(ignoreUnknown = true)
public class SshKeyEntry {

    @JsonProperty("id")         public String id;
    @JsonProperty("title")      public String title;
    @JsonProperty("public_key") public String publicKey;
    @JsonProperty("comment")    public String comment;
    @JsonProperty("algorithm")  public String algorithm;

    public Map<String, String> toFields() {
        return Map.of("public_key", publicKey == null ? "" : publicKey,
                      "comment",    comment == null ? "" : comment,
                      "algorithm",  algorithm == null ? "" : algorithm);
    }

    public static SshKeyEntry fromEntry(Entry e) {
        SshKeyEntry k = new SshKeyEntry();
        if (e == null) return k;
        k.id        = e.id;
        k.title     = e.title;
        k.publicKey = e.field("public_key");
        k.comment   = e.field("comment");
        k.algorithm = e.field("algorithm");
        return k;
    }

    @Override
    public String toString() {
        return "SshKeyEntry{id='" + id + "', title='" + title + "', algorithm='" + algorithm + "'}";
    }
}
