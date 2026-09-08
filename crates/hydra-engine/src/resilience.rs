//! Lock-Free Circuit Breaker for Network Gateway Ingestion Protection
//! ("Async Rust" - Chapter 9: Building Resilient Network Protocols & Fast-Fail)
//!
//! Prevents cascading system failures during network or downstream auth outages.
//! Rejects connections immediately (fast-fail) when the failure threshold is exceeded.

use std::sync::atomic::{AtomicU32, AtomicU64, Ordering};
use std::time::{Duration, Instant};

const STATE_CLOSED: u32 = 0;
const STATE_OPEN: u32 = 1;
const STATE_HALF_OPEN: u32 = 2;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CircuitState {
    Closed,
    Open,
    HalfOpen,
}

/// Fast-fail Atomic Circuit Breaker
pub struct StreamCircuitBreaker {
    state: AtomicU32,
    failure_count: AtomicU32,
    success_count: AtomicU32,
    failure_threshold: u32,
    success_threshold: u32,
    cooldown_ms: u64,
    last_state_change_ms: AtomicU64,
    start_instant: Instant,
}

impl StreamCircuitBreaker {
    pub fn new(failure_threshold: u32, cooldown: Duration) -> Self {
        Self {
            state: AtomicU32::new(STATE_CLOSED),
            failure_count: AtomicU32::new(0),
            success_count: AtomicU32::new(0),
            failure_threshold: failure_threshold.max(1),
            success_threshold: 3,
            cooldown_ms: cooldown.as_millis() as u64,
            last_state_change_ms: AtomicU64::new(0),
            start_instant: Instant::now(),
        }
    }

    fn now_ms(&self) -> u64 {
        self.start_instant.elapsed().as_millis() as u64
    }

    /// Checks if a new operation is permitted.
    /// Returns `true` if allowed, `false` if the circuit is OPEN (fast-fail).
    pub fn allow_request(&self) -> bool {
        let current_state = self.state.load(Ordering::Acquire);
        match current_state {
            STATE_CLOSED => true,
            STATE_HALF_OPEN => true,
            STATE_OPEN => {
                let now = self.now_ms();
                let last_change = self.last_state_change_ms.load(Ordering::Relaxed);
                if now.saturating_sub(last_change) >= self.cooldown_ms {
                    // Transition to Half-Open for probe requests
                    if self.state.compare_exchange(STATE_OPEN, STATE_HALF_OPEN, Ordering::AcqRel, Ordering::Relaxed).is_ok() {
                        self.success_count.store(0, Ordering::Relaxed);
                        return true;
                    }
                }
                false
            }
            _ => false,
        }
    }

    /// Records a successful connection/operation.
    pub fn record_success(&self) {
        let current_state = self.state.load(Ordering::Acquire);
        if current_state == STATE_HALF_OPEN {
            let count = self.success_count.fetch_add(1, Ordering::Relaxed) + 1;
            if count >= self.success_threshold {
                self.failure_count.store(0, Ordering::Relaxed);
                self.state.store(STATE_CLOSED, Ordering::Release);
            }
        } else if current_state == STATE_CLOSED {
            self.failure_count.store(0, Ordering::Relaxed);
        }
    }

    /// Records a failed connection/operation.
    pub fn record_failure(&self) {
        let failures = self.failure_count.fetch_add(1, Ordering::Relaxed) + 1;
        if failures >= self.failure_threshold {
            self.state.store(STATE_OPEN, Ordering::Release);
            self.last_state_change_ms.store(self.now_ms(), Ordering::Release);
        }
    }

    pub fn state(&self) -> CircuitState {
        match self.state.load(Ordering::Acquire) {
            STATE_CLOSED => CircuitState::Closed,
            STATE_OPEN => CircuitState::Open,
            STATE_HALF_OPEN => CircuitState::HalfOpen,
            _ => CircuitState::Closed,
        }
    }

    pub fn reset(&self) {
        self.state.store(STATE_CLOSED, Ordering::Release);
        self.failure_count.store(0, Ordering::Relaxed);
        self.success_count.store(0, Ordering::Relaxed);
    }
}
