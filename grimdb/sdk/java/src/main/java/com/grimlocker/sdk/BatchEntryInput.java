package com.grimlocker.sdk;

import java.util.Map;

/** Input for a single entry in a batch create operation. */
public class BatchEntryInput {
    public final String title;
    public final String category;
    public final Map<String, String> fields;

    public BatchEntryInput(String title, String category, Map<String, String> fields) {
        this.title = title;
        this.category = category;
        this.fields = fields;
    }
}
