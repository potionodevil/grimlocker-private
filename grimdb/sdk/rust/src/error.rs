use thiserror::Error;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ErrorCode {
    BusError        = -1,
    StorageError    = -2,
    NotFound        = -3,
    EntryNotFound   = -10,
    CategoryError   = -20,
    CreateFailed    = -30,
    UpdateFailed    = -31,
    DeleteFailed    = -32,
    ProtocolError   = -100,
    AuthRequired    = -101,
    PermissionDenied = -102,
    InvalidRequest  = -103,
    Timeout         = -104,
    Unknown         = 0,
}

impl From<i64> for ErrorCode {
    fn from(v: i64) -> Self {
        match v {
            -1   => Self::BusError,
            -2   => Self::StorageError,
            -3   => Self::NotFound,
            -10  => Self::EntryNotFound,
            -20  => Self::CategoryError,
            -30  => Self::CreateFailed,
            -31  => Self::UpdateFailed,
            -32  => Self::DeleteFailed,
            -100 => Self::ProtocolError,
            -101 => Self::AuthRequired,
            -102 => Self::PermissionDenied,
            -103 => Self::InvalidRequest,
            -104 => Self::Timeout,
            _    => Self::Unknown,
        }
    }
}

#[derive(Debug, Error)]
pub enum Error {
    #[error("connection failed: {0}")]
    Connect(String),

    #[error("daemon error {code:?}: {message}")]
    Daemon { code: ErrorCode, message: String },

    #[error("protocol error: {0}")]
    Protocol(String),

    #[error("json error: {0}")]
    Json(#[from] serde_json::Error),

    #[error("websocket error: {0}")]
    WebSocket(String),

    #[error("i/o error: {0}")]
    Io(#[from] std::io::Error),
}
