package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.grimlocker.sdk.Entry;
import java.util.Map;

@JsonIgnoreProperties(ignoreUnknown = true)
public class CertificateEntry {

    @JsonProperty("id")           public String id;
    @JsonProperty("title")        public String title;
    @JsonProperty("domain")       public String domain;
    @JsonProperty("certificate")  public String certificate;
    @JsonProperty("private_key")  public String privateKey;

    public Map<String, String> toFields() {
        return Map.of("domain",       domain == null ? "" : domain,
                      "certificate",  certificate == null ? "" : certificate,
                      "private_key",  privateKey == null ? "" : privateKey);
    }

    public static CertificateEntry fromEntry(Entry e) {
        CertificateEntry c = new CertificateEntry();
        if (e == null) return c;
        c.id          = e.id;
        c.title       = e.title;
        c.domain      = e.field("domain");
        c.certificate = e.field("certificate");
        c.privateKey  = e.field("private_key");
        return c;
    }

    @Override
    public String toString() {
        return "CertificateEntry{id='" + id + "', title='" + title + "', domain='" + domain + "'}";
    }
}
