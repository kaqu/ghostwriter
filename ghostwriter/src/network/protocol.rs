use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// Represents a protocol message exchanged over WebSockets.
#[derive(Debug, Serialize, Deserialize, PartialEq, Eq, Clone)]
pub struct Message {
    /// Unique identifier used for matching requests and responses.
    pub id: Uuid,
    /// Specific message variant.
    #[serde(flatten)]
    pub kind: MessageKind,
}

impl Message {
    /// Returns `true` if this message corresponds to the other by ID.
    #[allow(dead_code)]
    pub fn matches(&self, other: &Message) -> bool {
        self.id == other.id
    }
}

/// Variants of protocol messages.
#[derive(Debug, Serialize, Deserialize, PartialEq, Eq, Clone)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum MessageKind {
    /// Client initiates authentication with optional key.
    AuthRequest { key: Option<String> },
    /// Server replies with authentication result.
    AuthResponse {
        success: bool,
        reason: Option<String>,
    },
    /// Simple ping to keep connection alive.
    Ping,
    /// Response to `Ping` messages.
    Pong,
    /// Error message with human readable context.
    Error { context: String },
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_message_serialization() {
        let msg = Message {
            id: Uuid::nil(),
            kind: MessageKind::Ping,
        };
        let json = serde_json::to_string(&msg).expect("serialize");
        let de: Message = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(msg, de);
    }

    #[test]
    fn test_request_id_system() {
        let id = Uuid::new_v4();
        let req = Message {
            id,
            kind: MessageKind::AuthRequest {
                key: Some("k".into()),
            },
        };
        let res = Message {
            id,
            kind: MessageKind::AuthResponse {
                success: true,
                reason: None,
            },
        };
        assert!(req.matches(&res));
        let other = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::Pong,
        };
        assert!(!req.matches(&other));
    }

    #[test]
    fn test_authentication_flow() {
        let req = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::AuthRequest {
                key: Some("secret".into()),
            },
        };
        let json = serde_json::to_string(&req).unwrap();
        assert!(json.contains("auth_request"));
        let de: Message = serde_json::from_str(&json).unwrap();
        if let MessageKind::AuthRequest { key } = de.kind {
            assert_eq!(key.as_deref(), Some("secret"));
        } else {
            panic!("wrong variant");
        }
    }
}
