package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import java.util.Collections;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class SyncStatus {

    @JsonProperty("peers")        public List<SyncPeer> peers = Collections.emptyList();
    @JsonProperty("last_sync_at") public long lastSyncAt;

    @Override
    public String toString() {
        return "SyncStatus{peers=" + peers.size() + ", lastSyncAt=" + lastSyncAt + "}";
    }
}
