package gql

// ErrorCodeNames mapped GQL-Error-Codes zu lesbaren Namen.
// Wird von ErrorCodeName für Logging und SDK-Error-Messages verwendet.
var ErrorCodeNames = map[int32]string{
	-1:   "BUS_TIMEOUT",
	-2:   "INVALID_STORAGE_RESPONSE",
	-3:   "STORAGE_ERROR",
	-10:  "MISSING_ENTRY_ID",
	-11:  "ENTRY_NOT_FOUND",
	-20:  "CATEGORY_QUERY_FAILED",
	-30:  "CREATE_FAILED",
	-31:  "UPDATE_FAILED",
	-32:  "DELETE_FAILED",
	-100: "DISPATCHER_UNAVAILABLE",
	-101: "INVALID_FRAME",
	-102: "SCHEMA_VALIDATION",
	-103: "ACL_DENIED",
	-104: "NOT_A_QUERY_FRAME",
	-105: "DISPATCH_ERROR",
}

// ErrorCodeName gibt den symbolischen Namen für einen GQL-Error-Code zurück.
// Gibt "UNKNOWN" für unbekannte Codes zurück.
func ErrorCodeName(code int32) string {
	if name, ok := ErrorCodeNames[code]; ok {
		return name
	}
	return "UNKNOWN"
}
