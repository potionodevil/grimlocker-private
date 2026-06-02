package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonIgnoreProperties(ignoreUnknown = true)
public class UploadProgress {

    @JsonProperty("bytes_read")        public long bytesRead;
    @JsonProperty("total_size")        public long totalSize;
    @JsonProperty("manifest_block_id") public String manifestBlockId;

    public double percentComplete() {
        if (totalSize <= 0) return 0.0;
        return (double) bytesRead / (double) totalSize * 100.0;
    }

    @Override
    public String toString() {
        return "UploadProgress{bytesRead=" + bytesRead + ", totalSize=" + totalSize + ", manifestBlockId='" + manifestBlockId + "'}";
    }
}
