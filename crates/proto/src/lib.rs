//! Protocol types and serialization helpers for Ghostwriter.

use serde::{Deserialize, Serialize};

pub const PROTOCOL_VERSION: u16 = 1;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Envelope<T> {
    pub v: u16,
    #[serde(rename = "type")]
    pub ty: MessageType,
    pub data: T,
}

impl<T> Envelope<T> {
    pub fn new(ty: MessageType, data: T) -> Self {
        Self {
            v: PROTOCOL_VERSION,
            ty,
            data,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum MessageType {
    Hello,
    Auth,
    Open,
    Insert,
    Delete,
    Move,
    Select,
    Scroll,
    Resize,
    Search,
    GotoLine,
    DuplicateLine,
    DeleteLine,
    Save,
    RequestFrame,
    PickerAction,
    Ack,
    Frame,
    Dirty,
    Status,
    Dialog,
    Error,
    Ping,
    Pong,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Hello {
    pub client_name: String,
    pub client_ver: String,
    pub cols: u16,
    pub rows: u16,
    pub truecolor: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Ack {
    pub seq: u64,
    pub doc_v: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Range {
    pub from: u64,
    pub to: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Insert {
    pub pos: u64,
    pub text: String,
    pub seq: u64,
}

pub fn encode<T: Serialize>(envelope: &Envelope<T>) -> Result<Vec<u8>, rmp_serde::encode::Error> {
    rmp_serde::to_vec(envelope)
}

pub fn decode<'de, T: Deserialize<'de>>(
    bytes: &'de [u8],
) -> Result<Envelope<T>, rmp_serde::decode::Error> {
    rmp_serde::from_slice(bytes)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn envelope_roundtrip() {
        let hello = Hello {
            client_name: "ghostwriter".into(),
            client_ver: "1.0.0".into(),
            cols: 120,
            rows: 40,
            truecolor: true,
        };
        let env = Envelope::new(MessageType::Hello, hello.clone());
        let encoded = encode(&env).expect("encode");
        let decoded: Envelope<Hello> = decode(&encoded).expect("decode");
        assert_eq!(decoded.v, PROTOCOL_VERSION);
        assert_eq!(decoded.ty, MessageType::Hello);
        assert_eq!(decoded.data, hello);
    }
}
