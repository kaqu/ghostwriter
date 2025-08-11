use std::time::Duration;
use tokio::task::JoinHandle;
use tokio::time::sleep;

/// Debounce execution of a closure after a period of inactivity.
pub struct Debouncer {
    delay: Duration,
    handle: Option<JoinHandle<()>>,
}

impl Debouncer {
    /// Create a new `Debouncer` with the specified delay.
    pub fn new(delay: Duration) -> Self {
        Self {
            delay,
            handle: None,
        }
    }

    /// Trigger the debouncer with the given action.
    ///
    /// If called again before the delay elapses, the pending action is
    /// cancelled and rescheduled.
    pub fn call<F>(&mut self, action: F)
    where
        F: FnOnce() + Send + 'static,
    {
        if let Some(handle) = self.handle.take() {
            handle.abort();
        }
        let delay = self.delay;
        self.handle = Some(tokio::spawn(async move {
            sleep(delay).await;
            action();
        }));
    }
}

impl Default for Debouncer {
    fn default() -> Self {
        Self::new(Duration::from_millis(100))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Arc, Mutex};

    #[tokio::test]
    async fn debouncer_runs_once_after_delay() {
        let count = Arc::new(Mutex::new(0));
        let mut d = Debouncer::new(Duration::from_millis(50));
        let c = count.clone();
        d.call(move || {
            *c.lock().unwrap() += 1;
        });
        // call again before delay
        tokio::time::sleep(Duration::from_millis(20)).await;
        let c = count.clone();
        d.call(move || {
            *c.lock().unwrap() += 1;
        });
        tokio::time::sleep(Duration::from_millis(70)).await; // wait past delay
        assert_eq!(*count.lock().unwrap(), 1);
    }

    #[tokio::test]
    async fn default_delay_works() {
        let called = Arc::new(Mutex::new(false));
        let c = called.clone();
        let mut d = Debouncer::default();
        d.call(move || {
            *c.lock().unwrap() = true;
        });
        tokio::time::sleep(Duration::from_millis(120)).await;
        assert!(*called.lock().unwrap());
    }
}
