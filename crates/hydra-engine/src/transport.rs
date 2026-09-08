//! High-Performance Zero-Copy Frame Transport Hub
//! Based on "Rust Atomics and Locks" concurrency primitives and memory models.
//! Supports both pure CPU (Host RAM Arc) and Hybrid (GPU VRAM / CUDA IPC Handle) pipelines.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU32, AtomicU64, AtomicUsize, Ordering};
use atomic_wait::{wait, wake_all};

/// Cache-line aligned 64-bit atomic to eliminate False Sharing on L1/L2 caches.
#[repr(align(64))]
pub struct CacheAlignedAtomicU64 {
    pub value: AtomicU64,
}

impl CacheAlignedAtomicU64 {
    pub const fn new(val: u64) -> Self {
        Self {
            value: AtomicU64::new(val),
        }
    }
}

/// Cache-line aligned 32-bit atomic for Futex / address-based waiting.
#[repr(align(64))]
pub struct CacheAlignedAtomicU32 {
    pub value: AtomicU32,
}

impl CacheAlignedAtomicU32 {
    pub const fn new(val: u32) -> Self {
        Self {
            value: AtomicU32::new(val),
        }
    }
}

/// Storage backend for video frames: Host RAM slice or GPU VRAM descriptor.
#[derive(Debug, Clone)]
pub enum FrameStorage {
    /// Pure CPU pipeline: Zero-copy Arc-managed byte buffer in Host RAM.
    HostMemory {
        buffer: Arc<[u8]>,
    },
    /// Hybrid CPU+GPU pipeline: Handle pointing to VRAM (CUDA IPC, Vulkan DMA-BUF).
    GpuMemory {
        device_id: u32,
        ipc_handle: [u8; 64],
        dma_buf_fd: Option<i32>,
        pitch_stride: usize,
    },
}

/// Video frame descriptor with zero-copy reference counting.
#[derive(Clone)]
pub struct FramePayload {
    pub frame_id: u64,
    pub timestamp_us: u64,
    pub width: u32,
    pub height: u32,
    pub format: u32, // 1: RGB24, 2: BGR24, 3: NV12, 4: RGBA32
    pub storage: Arc<FrameStorage>,
}

impl FramePayload {
    /// Creates a Host RAM frame payload.
    pub fn new_host(
        frame_id: u64,
        timestamp_us: u64,
        width: u32,
        height: u32,
        format: u32,
        data: Vec<u8>,
    ) -> Self {
        Self {
            frame_id,
            timestamp_us,
            width,
            height,
            format,
            storage: Arc::new(FrameStorage::HostMemory {
                buffer: data.into(),
            }),
        }
    }

    /// Creates a GPU VRAM frame payload handle.
    pub fn new_gpu(
        frame_id: u64,
        timestamp_us: u64,
        width: u32,
        height: u32,
        format: u32,
        device_id: u32,
        ipc_handle: [u8; 64],
        pitch_stride: usize,
    ) -> Self {
        Self {
            frame_id,
            timestamp_us,
            width,
            height,
            format,
            storage: Arc::new(FrameStorage::GpuMemory {
                device_id,
                ipc_handle,
                dma_buf_fd: None,
                pitch_stride,
            }),
        }
    }
}

/// Slot in the circular broadcast ring buffer.
struct BroadcastSlot {
    sequence: AtomicU64,
    payload: parking_lot::RwLock<Option<FramePayload>>,
}

/// Multi-Consumer Broadcast Hub for WebSockets / Video Analytics.
/// Uses cache-line isolation, Release-Acquire happens-before semantics,
/// and OS-level Futex waiting via atomic-wait.
pub struct BroadcastHub {
    capacity: usize,
    slots: Vec<BroadcastSlot>,
    /// Producer head sequence (Cache-aligned to prevent false sharing with consumers)
    head_seq: CacheAlignedAtomicU64,
    /// Futex notification counter for address-based sleeping
    notify_futex: CacheAlignedAtomicU32,
    /// Telemetry counters using Ordering::Relaxed
    total_published: AtomicU64,
    total_dropped: AtomicU64,
    active_consumers: AtomicUsize,
    is_closed: AtomicBool,
}

impl BroadcastHub {
    pub fn new(capacity: usize) -> Self {
        let capacity = if capacity == 0 { 16 } else { capacity.next_power_of_two() };
        let mut slots = Vec::with_capacity(capacity);
        for _ in 0..capacity {
            slots.push(BroadcastSlot {
                sequence: AtomicU64::new(0),
                payload: parking_lot::RwLock::new(None),
            });
        }

        Self {
            capacity,
            slots,
            head_seq: CacheAlignedAtomicU64::new(0),
            notify_futex: CacheAlignedAtomicU32::new(0),
            total_published: AtomicU64::new(0),
            total_dropped: AtomicU64::new(0),
            active_consumers: AtomicUsize::new(0),
            is_closed: AtomicBool::new(false),
        }
    }

    /// Publishes a new frame to the ring buffer using Release ordering and wakes waiting consumers.
    pub fn publish(&self, frame: FramePayload) -> u64 {
        let seq = self.head_seq.value.load(Ordering::Relaxed) + 1;
        let slot_idx = (seq as usize) % self.capacity;
        let slot = &self.slots[slot_idx];

        // Store payload inside slot
        {
            let mut lock = slot.payload.write();
            *lock = Some(frame);
        }

        // Release ordering: Ensures frame payload is completely visible before sequence is updated
        slot.sequence.store(seq, Ordering::Release);
        self.head_seq.value.store(seq, Ordering::Release);

        // Relaxed metric update
        self.total_published.fetch_add(1, Ordering::Relaxed);

        // Futex notification: wake all sleeping consumer threads
        self.notify_futex.value.fetch_add(1, Ordering::Release);
        wake_all(&self.notify_futex.value);

        seq
    }

    /// Reads the latest available frame or blocks using Futex until a new frame arrives.
    /// Returns None if the hub is closed.
    pub fn wait_and_read(&self, last_seen_seq: u64) -> Option<FramePayload> {
        loop {
            if self.is_closed.load(Ordering::Acquire) {
                return None;
            }

            // Check-then-CAS principle: check with Relaxed/Acquire before waiting
            let current_head = self.head_seq.value.load(Ordering::Acquire);
            if current_head > last_seen_seq {
                let slot_idx = (current_head as usize) % self.capacity;
                let slot = &self.slots[slot_idx];

                // Check slot sequence with Acquire
                if slot.sequence.load(Ordering::Acquire) == current_head {
                    let lock = slot.payload.read();
                    if let Some(ref payload) = *lock {
                        return Some(payload.clone());
                    }
                }
            }

            // No new frame yet: suspend thread in OS Kernel via Futex (0% CPU)
            let current_futex = self.notify_futex.value.load(Ordering::Acquire);
            wait(&self.notify_futex.value, current_futex);
        }
    }

    /// Non-blocking read of the latest frame.
    pub fn try_read_latest(&self, last_seen_seq: u64) -> Option<FramePayload> {
        let current_head = self.head_seq.value.load(Ordering::Acquire);
        if current_head > last_seen_seq {
            let slot_idx = (current_head as usize) % self.capacity;
            let slot = &self.slots[slot_idx];

            if slot.sequence.load(Ordering::Acquire) == current_head {
                let lock = slot.payload.read();
                return (*lock).clone();
            }
        }
        None
    }

    /// Closes the hub and wakes all waiting consumers.
    pub fn close(&self) {
        self.is_closed.store(true, Ordering::Release);
        self.notify_futex.value.fetch_add(1, Ordering::Release);
        wake_all(&self.notify_futex.value);
    }

    /// Metrics using Ordering::Relaxed
    pub fn stats(&self) -> (u64, u64, usize) {
        (
            self.total_published.load(Ordering::Relaxed),
            self.total_dropped.load(Ordering::Relaxed),
            self.active_consumers.load(Ordering::Relaxed),
        )
    }
}
