package com.grimlocker.sdk.internal;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

/**
 * Low-level GQL binary frame encoder/decoder.
 * This is an internal implementation detail — application code should use
 * {@link com.grimlocker.sdk.GrimlockerClient} instead.
 *
 * Wire format (all multi-byte integers big-endian):
 * <pre>
 *   [0:4]   total_length  uint32   (length of everything after this header)
 *   [4]     opcode        uint8    (0x10 = query, 0x11 = result, 0x12 = error)
 *   [5:n]   payload       bytes
 * </pre>
 *
 * Query payload layout:
 * <pre>
 *   [0]     field_count   uint8
 *   [1:n]   operation     length-prefixed string (uint16 len + bytes)
 *   [n:m]   namespace     length-prefixed string
 *   [m:p]   entry_id      length-prefixed string
 *   [p:q]   category      length-prefixed string
 *   [q:r]   title         length-prefixed string
 *   [r:s]   fields        field_count × (key_lp_string + value_lp_string)
 *   [s:s+4] limit         uint32
 *   [s+4:]  offset        uint32
 * </pre>
 */
public final class GQLFrame {

    public static final byte OPCODE_QUERY  = 0x10;
    public static final byte OPCODE_RESULT = 0x11;
    public static final byte OPCODE_ERROR  = 0x12;

    private GQLFrame() {}

    /**
     * Encodes a GQL query into a wire-ready byte array (4-byte length prefix included).
     */
    public static byte[] encodeQuery(
            String operation,
            String namespace,
            String entryId,
            String category,
            String title,
            Map<String, String> fields,
            int limit,
            int offset) {

        if (fields == null) fields = new HashMap<>();

        // Calculate payload size
        int payloadSize = 1 // field_count
            + lpSize(operation)
            + lpSize(namespace)
            + lpSize(entryId)
            + lpSize(category)
            + lpSize(title)
            + 4  // limit
            + 4; // offset
        for (Map.Entry<String, String> kv : fields.entrySet()) {
            payloadSize += lpSize(kv.getKey()) + lpSize(kv.getValue());
        }

        ByteBuffer buf = ByteBuffer.allocate(4 + 1 + payloadSize).order(ByteOrder.BIG_ENDIAN);
        buf.putInt(1 + payloadSize); // total_length (after this header)
        buf.put(OPCODE_QUERY);       // opcode

        // field_count
        buf.put((byte) Math.min(fields.size(), 255));
        // fields
        putLpString(buf, operation);
        putLpString(buf, namespace);
        putLpString(buf, entryId);
        putLpString(buf, category);
        putLpString(buf, title);
        for (Map.Entry<String, String> kv : fields.entrySet()) {
            putLpString(buf, kv.getKey());
            putLpString(buf, kv.getValue());
        }
        buf.putInt(limit);
        buf.putInt(offset);

        return buf.array();
    }

    /** Returns the byte length of a length-prefixed string field: 2 + str.length(). */
    private static int lpSize(String s) {
        if (s == null) return 2;
        return 2 + s.getBytes(StandardCharsets.UTF_8).length;
    }

    private static void putLpString(ByteBuffer buf, String s) {
        if (s == null) {
            buf.putShort((short) 0);
            return;
        }
        byte[] bytes = s.getBytes(StandardCharsets.UTF_8);
        buf.putShort((short) bytes.length);
        buf.put(bytes);
    }

    /** Reads the opcode from a raw response frame. */
    public static byte readOpcode(byte[] frame) {
        if (frame == null || frame.length < 5) {
            throw new IllegalArgumentException("frame too short");
        }
        return frame[4];
    }

    /** Extracts the JSON payload from a raw response frame. */
    public static byte[] readPayload(byte[] frame) {
        if (frame == null || frame.length < 5) {
            throw new IllegalArgumentException("frame too short");
        }
        int payloadLen = frame.length - 5;
        byte[] payload = new byte[payloadLen];
        System.arraycopy(frame, 5, payload, 0, payloadLen);
        return payload;
    }

    /**
     * Encodes a JSON payload command for non-entry operations (file vault, workspace, sync, audit, tools).
     * The JSON payload is sent as a single LP-string field with key "payload" after the
     * standard GQL query fields.
     */
    public static byte[] encodeJsonPayload(String operation, String namespace, String jsonPayload) {
        if (jsonPayload == null) jsonPayload = "{}";

        int fieldCount = 1;
        byte[] jsonBytes = jsonPayload.getBytes(StandardCharsets.UTF_8);

        int payloadSize = 1 // field_count
            + lpSize(operation)
            + lpSize(namespace)
            + lpSize("")  // entryId
            + lpSize("")  // category
            + lpSize("")  // title
            + lpSize("payload") + (2 + jsonBytes.length) // key=payload, value=jsonPayload
            + 4  // limit
            + 4; // offset

        ByteBuffer buf = ByteBuffer.allocate(4 + 1 + payloadSize).order(ByteOrder.BIG_ENDIAN);
        buf.putInt(1 + payloadSize);
        buf.put(OPCODE_QUERY);

        buf.put((byte) fieldCount);
        putLpString(buf, operation);
        putLpString(buf, namespace);
        putLpString(buf, "");
        putLpString(buf, "");
        putLpString(buf, "");
        putLpString(buf, "payload");
        buf.putShort((short) jsonBytes.length);
        buf.put(jsonBytes);
        buf.putInt(50);
        buf.putInt(0);

        return buf.array();
    }
}
