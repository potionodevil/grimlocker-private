package com.grimlocker.sdk;

import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Fluent builder for GQL queries.
 *
 * <pre>{@code
 * List<Entry> entries = client.listEntries()
 *     .namespace("default")
 *     .limit(20)
 *     .execute();
 * }</pre>
 */
public class QueryBuilder {

    private final GrimlockerClient client;
    private final String operation;

    private String namespace  = "default";
    private String entryId    = "";
    private String category   = "";
    private String title      = "";
    private Map<String, String> fields = Collections.emptyMap();
    private int limit  = 50;
    private int offset = 0;

    QueryBuilder(GrimlockerClient client, String operation) {
        this.client    = client;
        this.operation = operation;
    }

    public QueryBuilder namespace(String ns)   { this.namespace = ns; return this; }
    public QueryBuilder entryId(String id)     { this.entryId   = id; return this; }
    public QueryBuilder category(String cat)   { this.category  = cat; return this; }
    public QueryBuilder title(String t)        { this.title     = t; return this; }
    public QueryBuilder limit(int n)           { this.limit     = n; return this; }
    public QueryBuilder offset(int n)          { this.offset    = n; return this; }

    public QueryBuilder field(String key, String value) {
        if (this.fields == Collections.<String,String>emptyMap()) {
            this.fields = new HashMap<>();
        }
        this.fields.put(key, value);
        return this;
    }

    public QueryBuilder fields(Map<String, String> m) {
        this.fields = new HashMap<>(m);
        return this;
    }

    /**
     * Executes the query and returns the result entries.
     *
     * @throws GrimlockerException on daemon error or connection failure
     */
    public List<Entry> execute() {
        return client.executeQuery(operation, namespace, entryId, category, title, fields, limit, offset);
    }

    /**
     * Executes and returns the first entry, or null if the result is empty.
     *
     * @throws GrimlockerException on daemon error or connection failure
     */
    public Entry executeOne() {
        List<Entry> results = execute();
        return results.isEmpty() ? null : results.get(0);
    }
}
