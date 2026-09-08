//! Low-Level Synchronization Primitives based on "Rust Atomics and Locks" (Mara Bos):
//! 1. AtomicSemaphore: Backpressure control using single AtomicU32 + Futex.
//! 2. HybridMutex: 3-State adaptive Mutex (100x std::hint::spin_loop before Futex syscall).
//! 3. ParkingTable: Global address-based wait queue table (Parking Lot architecture).

use std::cell::UnsafeCell;
use std::collections::HashMap;
use std::hint::spin_loop;
use std::ops::{Deref, DerefMut};
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;
use crate::transport::{futex_wait_bitset, futex_wake_bitset, BIT_ALL};

// ---------------------------------------------------------------------------
// 1. Atomic Semaphore (Backpressure & Bounded Ring Buffer Control)
// ---------------------------------------------------------------------------

/// Counting Semaphore using a single AtomicU32 with Futex wait/wake.
/// Prevents producers from overflowing ring buffers when readers lag behind.
pub struct AtomicSemaphore {
    permits: AtomicU32,
    pub max_permits: u32,
}

impl AtomicSemaphore {
    pub const fn new(initial_permits: u32) -> Self {
        Self {
            permits: AtomicU32::new(initial_permits),
            max_permits: initial_permits,
        }
    }

    /// Acquires a permit. If none available, blocks in Futex until released (Backpressure).
    pub fn acquire(&self, timeout: Option<Duration>) -> bool {
        let start = std::time::Instant::now();
        loop {
            let current = self.permits.load(Ordering::Relaxed);
            if current > 0 {
                // Weak compare_exchange to optimize for ARM64 (LL/SC) and x86_64
                if self.permits.compare_exchange_weak(
                    current,
                    current - 1,
                    Ordering::Acquire,
                    Ordering::Relaxed,
                ).is_ok() {
                    return true;
                }
                continue;
            }

            if let Some(t) = timeout {
                if start.elapsed() >= t {
                    return false;
                }
            }

            // Block in OS Kernel via Futex
            unsafe {
                futex_wait_bitset(&self.permits, 0, BIT_ALL, timeout);
            }
        }
    }

    /// Releases a permit and wakes a waiting producer thread.
    pub fn release(&self) {
        let prev = self.permits.fetch_add(1, Ordering::Release);
        if prev == 0 {
            unsafe {
                futex_wake_bitset(&self.permits, BIT_ALL);
            }
        }
    }

    pub fn available(&self) -> u32 {
        self.permits.load(Ordering::Relaxed)
    }
}

// ---------------------------------------------------------------------------
// 2. 3-State Adaptive Hybrid Mutex (Spin-Wait -> Futex)
// ---------------------------------------------------------------------------

const STATE_UNLOCKED: u32 = 0;
const STATE_LOCKED: u32 = 1;
const STATE_LOCKED_WITH_WAITERS: u32 = 2;

/// 3-State Adaptive Hybrid Mutex from Chapter 9 of "Rust Atomics and Locks":
/// 1. State 0: Unlocked
/// 2. State 1: Locked (No sleeping waiters)
/// 3. State 2: Locked (Waiters sleeping in Futex)
/// Spins up to 100 times with `std::hint::spin_loop()` before inducing a 1500ns Futex syscall.
pub struct HybridMutex<T> {
    state: AtomicU32,
    data: UnsafeCell<T>,
}

unsafe impl<T: Send> Sync for HybridMutex<T> {}
unsafe impl<T: Send> Send for HybridMutex<T> {}

impl<T> HybridMutex<T> {
    pub const fn new(data: T) -> Self {
        Self {
            state: AtomicU32::new(STATE_UNLOCKED),
            data: UnsafeCell::new(data),
        }
    }

    pub fn lock(&self) -> HybridMutexGuard<'_, T> {
        // Fast-path: try to acquire uncontended lock
        if self.state.compare_exchange(
            STATE_UNLOCKED,
            STATE_LOCKED,
            Ordering::Acquire,
            Ordering::Relaxed,
        ).is_err() {
            self.lock_contended();
        }

        HybridMutexGuard { mutex: self }
    }

    fn lock_contended(&self) {
        let mut spin_count = 0;

        // Phase 1: Adaptive Spin-Wait (up to 100 iterations with processor hint)
        while self.state.load(Ordering::Relaxed) == STATE_LOCKED && spin_count < 100 {
            spin_loop();
            spin_count += 1;
        }

        // Try fast acquire again after spinning
        if self.state.compare_exchange(
            STATE_UNLOCKED,
            STATE_LOCKED,
            Ordering::Acquire,
            Ordering::Relaxed,
        ).is_ok() {
            return;
        }

        // Phase 2: Escalate to Futex Kernel Sleep
        while self.state.swap(STATE_LOCKED_WITH_WAITERS, Ordering::Acquire) != STATE_UNLOCKED {
            unsafe {
                futex_wait_bitset(&self.state, STATE_LOCKED_WITH_WAITERS, BIT_ALL, None);
            }
        }
    }

    fn unlock(&self) {
        // If state was STATE_LOCKED (1), no waiters were sleeping, simple store is enough
        if self.state.swap(STATE_UNLOCKED, Ordering::Release) == STATE_LOCKED_WITH_WAITERS {
            // Wake one sleeping thread
            unsafe {
                libc::syscall(
                    libc::SYS_futex,
                    &self.state as *const AtomicU32 as *const u32,
                    libc::FUTEX_WAKE,
                    1i32,
                    std::ptr::null::<libc::timespec>(),
                    std::ptr::null::<u32>(),
                    0u32,
                );
            }
        }
    }
}

pub struct HybridMutexGuard<'a, T> {
    mutex: &'a HybridMutex<T>,
}

impl<'a, T> Deref for HybridMutexGuard<'a, T> {
    type Target = T;
    fn deref(&self) -> &T {
        unsafe { &*self.mutex.data.get() }
    }
}

impl<'a, T> DerefMut for HybridMutexGuard<'a, T> {
    fn deref_mut(&mut self) -> &mut T {
        unsafe { &mut *self.mutex.data.get() }
    }
}

impl<'a, T> Drop for HybridMutexGuard<'a, T> {
    fn drop(&mut self) {
        self.mutex.unlock();
    }
}

// ---------------------------------------------------------------------------
// 3. Address-Based Parking Table (Parking Lot Pattern)
// ---------------------------------------------------------------------------

struct WaitEntry {
    notified: AtomicU32,
}

/// Global/Centralized Parking Table from Chapter 10.
/// Allows thousands of WebSocket connections to synchronize without embedding
/// individual Mutexes/Condvars in each connection struct.
pub struct ParkingTable {
    buckets: [parking_lot::Mutex<HashMap<usize, Vec<std::sync::Arc<WaitEntry>>>>; 16],
}

impl ParkingTable {
    pub fn new() -> Self {
        Self {
            buckets: std::array::from_fn(|_| parking_lot::Mutex::new(HashMap::new())),
        }
    }

    fn bucket_idx(&self, key: usize) -> usize {
        (key ^ (key >> 4)) & 0x0F
    }

    /// Parks the current thread on a specific memory address key
    pub fn park(&self, key: usize, timeout: Option<Duration>) -> bool {
        let entry = std::sync::Arc::new(WaitEntry {
            notified: AtomicU32::new(0),
        });

        let idx = self.bucket_idx(key);
        {
            let mut map = self.buckets[idx].lock();
            map.entry(key).or_insert_with(Vec::new).push(std::sync::Arc::clone(&entry));
        }

        // Wait on the entry's private futex
        let start = std::time::Instant::now();
        while entry.notified.load(Ordering::Acquire) == 0 {
            if let Some(t) = timeout {
                if start.elapsed() >= t {
                    return false;
                }
            }
            unsafe {
                futex_wait_bitset(&entry.notified, 0, BIT_ALL, timeout);
            }
        }
        true
    }

    /// Unparks/wakes all threads parked on a specific memory address key
    pub fn unpark_all(&self, key: usize) -> usize {
        let idx = self.bucket_idx(key);
        let entries = {
            let mut map = self.buckets[idx].lock();
            map.remove(&key)
        };

        if let Some(waiters) = entries {
            let count = waiters.len();
            for w in waiters {
                w.notified.store(1, Ordering::Release);
                unsafe {
                    futex_wake_bitset(&w.notified, BIT_ALL);
                }
            }
            count
        } else {
            0
        }
    }
}
