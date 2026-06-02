package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.grimlocker.sdk.Entry;
import java.util.Map;

@JsonIgnoreProperties(ignoreUnknown = true)
public class FileEntry {

    @JsonProperty("id")                public String id;
    @JsonProperty("manifest_block_id") public String manifestBlockId;
    @JsonProperty("file_name")         public String fileName;
    @JsonProperty("mime_type")         public String mimeType;
    @JsonProperty("total_size")        public long totalSize;
    @JsonProperty("folder_id")         public String folderId;
    @JsonProperty("created_at")        public long createdAt;

    public Map<String, String> toFields() {
        return Map.of("manifest_block_id", manifestBlockId == null ? "" : manifestBlockId,
                      "file_name",         fileName == null ? "" : fileName,
                      "mime_type",         mimeType == null ? "" : mimeType,
                      "total_size",        String.valueOf(totalSize),
                      "folder_id",         folderId == null ? "" : folderId);
    }

    public static FileEntry fromEntry(Entry e) {
        FileEntry f = new FileEntry();
        if (e == null) return f;
        f.id              = e.id;
        f.manifestBlockId = e.field("manifest_block_id");
        f.fileName        = e.field("file_name");
        f.mimeType        = e.field("mime_type");
        try { f.totalSize = Long.parseLong(e.field("total_size")); } catch (NumberFormatException ignored) {}
        f.folderId        = e.field("folder_id");
        f.createdAt       = e.createdAt;
        return f;
    }

    @Override
    public String toString() {
        return "FileEntry{manifestBlockId='" + manifestBlockId + "', fileName='" + fileName + "', size=" + totalSize + "}";
    }
}
