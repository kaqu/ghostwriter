use serde::{Deserialize, Serialize};
use std::fs;
use std::io;
use std::path::Path;

use crate::error::Result;

#[derive(Debug, Serialize, Deserialize, Default, PartialEq, Eq)]
pub struct AppState {
    pub recent_files: Vec<String>,
}

#[allow(dead_code)]
pub struct StateManager;

#[allow(dead_code)]
impl StateManager {
    pub fn load(path: &Path) -> Result<AppState> {
        match fs::read_to_string(path) {
            Ok(data) => match serde_json::from_str(&data) {
                Ok(state) => Ok(state),
                Err(_) => {
                    log::warn!("corrupted state file {:?}, resetting", path);
                    Ok(AppState::default())
                }
            },
            Err(e) if e.kind() == io::ErrorKind::NotFound => Ok(AppState::default()),
            Err(e) => Err(e.into()),
        }
    }

    pub fn save(path: &Path, state: &AppState) -> Result<()> {
        let data = serde_json::to_vec(state)?;
        fs::write(path, data)?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_state_corruption_recovery() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("state.json");
        fs::write(&path, b"not json").unwrap();
        let state = StateManager::load(&path).unwrap();
        assert!(state.recent_files.is_empty());
    }
}
