package com.grimlocker.sdk.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonIgnoreProperties(ignoreUnknown = true)
public class FolderItem {

    @JsonProperty("id")         public String id;
    @JsonProperty("name")       public String name;
    @JsonProperty("parent_id")  public String parentId;
    @JsonProperty("created_at") public long createdAt;
    @JsonProperty("updated_at") public long updatedAt;

    @Override
    public String toString() {
        return "FolderItem{id='" + id + "', name='" + name + "', parentId='" + parentId + "'}";
    }
}
