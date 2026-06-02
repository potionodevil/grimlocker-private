use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Workspace {
    pub id:         String,
    pub name:       String,
    pub is_default: bool,
}
