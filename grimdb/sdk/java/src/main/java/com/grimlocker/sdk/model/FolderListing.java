package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import java.util.Collections;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class FolderListing {

    @JsonProperty("folder_id")   public String folderId;
    @JsonProperty("folder_name") public String folderName;
    @JsonProperty("files")       public List<FileEntry> files = Collections.emptyList();
    @JsonProperty("folders")     public List<FolderItem> folders = Collections.emptyList();

    @Override
    public String toString() {
        return "FolderListing{folderId='" + folderId + "', files=" + files.size() + ", folders=" + folders.size() + "}";
    }
}
