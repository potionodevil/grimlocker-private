package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonIgnoreProperties(ignoreUnknown = true)
public class SyncPeer {

    @JsonProperty("id")        public String id;
    @JsonProperty("name")      public String name;
    @JsonProperty("address")   public String address;
    @JsonProperty("connected") public boolean connected;
    @JsonProperty("last_seen") public long lastSeen;

    @Override
    public String toString() {
        return "SyncPeer{id='" + id + "', name='" + name + "', connected=" + connected + "}";
    }
}
