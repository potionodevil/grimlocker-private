use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct AuditEvent {
    pub timestamp:  i64,
    pub level:      String,
    pub module:     String,
    pub message:    String,
    pub subject_id: Option<String>,
}
