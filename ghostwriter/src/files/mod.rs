// files module

pub mod file_lock;
pub mod file_manager;
pub mod file_watcher;

pub fn hello_files() {
    println!("Hello from files module!");
    let path = std::env::temp_dir().join("gw_lock_test");
    if let Ok(lock) = file_lock::FileLock::acquire(&path, std::time::Duration::from_millis(10)) {
        let _ = lock.readonly();
    }
}
