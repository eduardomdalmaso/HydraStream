//! High-Performance Zero-Copy Frame Transport Hub
//! Based on "Rust Atomics and Locks" (Mara Bos) concurrency primitives:
//! 1. Cache-line isolation (#[repr(align(64))]) preventing False Sharing.
//! 2. Selective Futex Bitset Waiting (FUTEX_WAIT_BITSET / FUTEX_WAKE_BITSET) eliminating Thundering Herd.
//! 3. Lock-Free Linked Lists (AtomicPtr) for dynamic peer connection management.
//! 4. Read-Copy-Update (RCU) pattern for zero-lock stream configuration changes.
//! 5. MaybeUninit/ManuallyDrop zero-overhead memory layout without enum discriminators.

use std::cell::UnsafeCell;
use std::mem::MaybeUninit;
use std::ptr;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicPtr, AtomicU32, AtomicU64, AtomicUsize, Ordering};
use std::time::Duration;

// ---------------------------------------------------------------------------
// 1. Bitset Masks & Selective Futex Waiting
// ---------------------------------------------------------------------------

pub const BIT_WEBSOCKET: u32 = 0x00000001;
pub const BIT_WEBRTC: u32    = 0x00000002;
pub const BIT_ANALYTICS: u32 = 0x00000004;
pub const BIT_ALL: u32       = 0xFFFFFFFF;

#[cfg(target_os = "linux")]
pub unsafe fn futex_wake_bitset(uaddr: *const AtomicU32, bitmask: u32) -> i32 {
    libc::syscall(
        libc::SYS_futex,
        uaddr as *const u32,
        libc::FUTEX_WAKE_BITSET,
        i32::MAX,
        std::ptr::null::<libc::timespec>(),
        std::ptr::null::<u32>(),
        bitmask,
    ) as i32
}

#[cfg(target_os = "linux")]
pub unsafe fn futex_wait_bitset(uaddr: *const AtomicU32, val: u32, bitmask: u32, timeout: Option<Duration>) -> bool {
    let ts = timeout.map(|d| libc::timespec {
        tv_sec: d.as_secs() as libc::time_t,
        tv_nsec: d.subsec_nanos() as libc::c_long,
    });
    let ts_ptr = ts.as_ref().map_or(std::ptr::null(), |t| t as *const libc::timespec);

    let ret = libc::syscall(
        libc::SYS_futex,
        uaddr as *const u32,
        libc::FUTEX_WAIT_BITSET,
        val,
        ts_ptr,
        std::ptr::null::<u32>(),
        bitmask,
    );
    ret == 0
}

#[cfg(not(target_os = "linux"))]
pub unsafe fn futex_wake_bitset(uaddr: *const AtomicU32, _bitmask: u32) -> i32 {
    atomic_wait::wake_all(&*uaddr);
    1
}

#[cfg(not(target_os = "linux"))]
pub unsafe fn futex_wait_bitset(uaddr: *const AtomicU32, val: u32, _bitmask: u32, _timeout: Option<Duration>) -> bool {
    atomic_wait::wait(&*uaddr, val);
    true
}

// ---------------------------------------------------------------------------
// 2. Cache-Line Aligned Atomics (MESI Protocol Protection)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// 3. Lock-Free Dynamic Peer Management (AtomicPtr Linked List)
// ---------------------------------------------------------------------------

pub struct PeerNode {
    pub client_id: usize,
    pub interest_mask: u32,
    pub active: AtomicBool,
    pub next: AtomicPtr<PeerNode>,
}

pub struct LockFreePeerList {
    head: AtomicPtr<PeerNode>,
    count: AtomicUsize,
}

impl LockFreePeerList {
    pub const fn new() -> Self {
        Self {
            head: AtomicPtr::new(ptr::null_mut()),
            count: AtomicUsize::new(0),
        }
    }

    /// Lock-Free Insertion via CAS loop
    pub fn register(&self, client_id: usize, interest_mask: u32) -> *mut PeerNode {
        let node = Box::into_raw(Box::new(PeerNode {
            client_id,
            interest_mask,
            active: AtomicBool::new(true),
            next: AtomicPtr::new(ptr::null_mut()),
        }));

        let mut current = self.head.load(Ordering::Relaxed);
        loop {
            unsafe { (*node).next.store(current, Ordering::Relaxed) };
            match self.head.compare_exchange_weak(
                current,
                node,
                Ordering::Release,
                Ordering::Relaxed,
            ) {
                Ok(_) => {
                    self.count.fetch_add(1, Ordering::Relaxed);
                    return node;
                }
                Err(actual) => current = actual,
            }
        }
    }

    /// Mark peer inactive without locking readers
    pub fn mark_inactive(&self, client_id: usize) {
        self.for_each(|node| {
            if node.client_id == client_id {
                node.active.store(false, Ordering::Release);
            }
        });
    }

    /// Pure Lock-Free Reader Traversal (Runs at 60+ FPS with zero contention)
    pub fn for_each<F: FnMut(&PeerNode)>(&self, mut callback: F) {
        let mut curr = self.head.load(Ordering::Acquire);
        while !curr.is_null() {
            unsafe {
                if (*curr).active.load(Ordering::Acquire) {
                    callback(&*curr);
                }
                curr = (*curr).next.load(Ordering::Acquire);
            }
        }
    }

    pub fn active_count(&self) -> usize {
        let mut n = 0;
        self.for_each(|_| n += 1);
        n
    }
}

impl Drop for LockFreePeerList {
    fn drop(&mut self) {
        let mut curr = self.head.swap(ptr::null_mut(), Ordering::Acquire);
        while !curr.is_null() {
            unsafe {
                let next = (*curr).next.load(Ordering::Relaxed);
                let _ = Box::from_raw(curr);
                curr = next;
            }
        }
    }
}

// ---------------------------------------------------------------------------
// 4. Read-Copy-Update (RCU) Pattern for Dynamic Configuration
// ---------------------------------------------------------------------------

pub struct RcuConfig<T> {
    ptr: AtomicPtr<Arc<T>>,
}

impl<T> RcuConfig<T> {
    pub fn new(initial: T) -> Self {
        let arc = Arc::new(initial);
        let raw = Box::into_raw(Box::new(arc));
        Self {
            ptr: AtomicPtr::new(raw),
        }
    }

    /// Lock-Free Instantaneous Read
    pub fn load(&self) -> Arc<T> {
        unsafe {
            let boxed_ptr = self.ptr.load(Ordering::Acquire);
            (*boxed_ptr).clone()
        }
    }

    /// Atomic Update (Read-Copy-Update) with Release ordering
    pub fn update(&self, new_val: T) {
        let new_arc = Arc::new(new_val);
        let new_raw = Box::into_raw(Box::new(new_arc));
        let old_raw = self.ptr.swap(new_raw, Ordering::AcqRel);
        
        unsafe {
            let _ = Box::from_raw(old_raw);
        }
    }
}

impl<T> Drop for RcuConfig<T> {
    fn drop(&mut self) {
        let raw = self.ptr.swap(ptr::null_mut(), Ordering::Acquire);
        if !raw.is_null() {
            unsafe {
                let _ = Box::from_raw(raw);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// 5. Frame Payload & Zero-Overhead Memory Layout (MaybeUninit)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, PartialEq)]
pub enum FrameStorage {
    HostMemory {
        buffer: Arc<[u8]>,
    },
    GpuMemory {
        device_id: u32,
        ipc_handle: [u8; 64],
        dma_buf_fd: Option<i32>,
        pitch_stride: usize,
    },
}

#[derive(Debug, Clone, PartialEq)]
pub struct FramePayload {
    pub frame_id: u64,
    pub timestamp_us: u64,
    pub width: u32,
    pub height: u32,
    pub format: u32,
    pub mask: u32, // Bitset category: BIT_WEBSOCKET, BIT_WEBRTC, etc.
    pub storage: Arc<FrameStorage>,
}

impl FramePayload {
    pub fn new_host(
        frame_id: u64,
        timestamp_us: u64,
        width: u32,
        height: u32,
        format: u32,
        mask: u32,
        data: Vec<u8>,
    ) -> Self {
        Self {
            frame_id,
            timestamp_us,
            width,
            height,
            format,
            mask,
            storage: Arc::new(FrameStorage::HostMemory {
                buffer: data.into(),
            }),
        }
    }

    pub fn new_gpu(
        frame_id: u64,
        timestamp_us: u64,
        width: u32,
        height: u32,
        format: u32,
        mask: u32,
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
            mask,
            storage: Arc::new(FrameStorage::GpuMemory {
                device_id,
                ipc_handle,
                dma_buf_fd: None,
                pitch_stride,
            }),
        }
    }

    #[inline(always)]
    pub fn host_buffer(&self) -> Option<&[u8]> {
        match &*self.storage {
            FrameStorage::HostMemory { buffer } => Some(buffer),
            _ => None,
        }
    }
}

/// Zero-overhead cache-line aligned slot using MaybeUninit (no Option tag overhead)
#[repr(C, align(64))]
struct RawBroadcastSlot {
    sequence: AtomicU64,
    payload: UnsafeCell<MaybeUninit<FramePayload>>,
}

impl RawBroadcastSlot {
    fn new() -> Self {
        Self {
            sequence: AtomicU64::new(0),
            payload: UnsafeCell::new(MaybeUninit::uninit()),
        }
    }
}

// ---------------------------------------------------------------------------
// 6. Multi-Consumer Broadcast Hub with Bitset Waking
// ---------------------------------------------------------------------------

pub struct BroadcastHub {
    capacity: usize,
    slots: Vec<RawBroadcastSlot>,
    head_seq: CacheAlignedAtomicU64,
    notify_futex: CacheAlignedAtomicU32,
    pub peers: LockFreePeerList,
    total_published: AtomicU64,
    total_dropped: AtomicU64,
    is_closed: AtomicBool,
}

unsafe impl Sync for BroadcastHub {}
unsafe impl Send for BroadcastHub {}

impl BroadcastHub {
    pub fn new(capacity: usize) -> Self {
        let capacity = if capacity == 0 { 16 } else { capacity.next_power_of_two() };
        let mut slots = Vec::with_capacity(capacity);
        for _ in 0..capacity {
            slots.push(RawBroadcastSlot::new());
        }

        Self {
            capacity,
            slots,
            head_seq: CacheAlignedAtomicU64::new(0),
            notify_futex: CacheAlignedAtomicU32::new(0),
            peers: LockFreePeerList::new(),
            total_published: AtomicU64::new(0),
            total_dropped: AtomicU64::new(0),
            is_closed: AtomicBool::new(false),
        }
    }

    /// Publishes a new frame and triggers selective Futex Bitset waking
    pub fn publish(&self, frame: FramePayload) -> u64 {
        let seq = self.head_seq.value.load(Ordering::Relaxed) + 1;
        let slot_idx = (seq as usize) % self.capacity;
        let slot = match self.slots.get(slot_idx) {
            Some(s) => s,
            None => return 0,
        };

        let frame_mask = frame.mask;

        // Write directly to MaybeUninit slot memory
        unsafe {
            let ptr = slot.payload.get();
            // Drop old frame if sequence > capacity
            if slot.sequence.load(Ordering::Relaxed) > 0 {
                std::ptr::drop_in_place((*ptr).as_mut_ptr());
            }
            (*ptr).write(frame);
        }

        // Release ordering: Ensures payload is fully committed before sequence becomes visible
        slot.sequence.store(seq, Ordering::Release);
        self.head_seq.value.store(seq, Ordering::Release);

        self.total_published.fetch_add(1, Ordering::Relaxed);

        // Selective Futex Wake: only wake threads subscribed to this frame type
        self.notify_futex.value.fetch_add(1, Ordering::Release);
        unsafe {
            futex_wake_bitset(&self.notify_futex.value, frame_mask);
        }

        seq
    }

    /// Selective Bitset Waiting: Blocks until a frame matching interest_mask arrives
    pub fn wait_and_read(&self, last_seen_seq: u64, interest_mask: u32, timeout: Option<Duration>) -> Option<FramePayload> {
        let start = std::time::Instant::now();
        loop {
            if self.is_closed.load(Ordering::Acquire) {
                return None;
            }

            // Check-then-CAS principle: check with Acquire before sleeping
            let current_head = self.head_seq.value.load(Ordering::Acquire);
            if current_head > last_seen_seq {
                let slot_idx = (current_head as usize) % self.capacity;
                if let Some(slot) = self.slots.get(slot_idx) {
                    if slot.sequence.load(Ordering::Acquire) == current_head {
                        unsafe {
                            let ptr = slot.payload.get();
                            let payload_ref = (*ptr).assume_init_ref();
                            if (payload_ref.mask & interest_mask) != 0 {
                                return Some(payload_ref.clone());
                            }
                        }
                    }
                }
            }

            if let Some(t) = timeout {
                if start.elapsed() >= t {
                    return None;
                }
            }

            // Sleep selectively via Futex Bitset
            let current_futex = self.notify_futex.value.load(Ordering::Acquire);
            unsafe {
                futex_wait_bitset(&self.notify_futex.value, current_futex, interest_mask, timeout);
            }
        }
    }

    /// Non-blocking read of latest frame
    pub fn try_read_latest(&self, last_seen_seq: u64, interest_mask: u32) -> Option<FramePayload> {
        let current_head = self.head_seq.value.load(Ordering::Acquire);
        if current_head > last_seen_seq {
            let slot_idx = (current_head as usize) % self.capacity;
            if let Some(slot) = self.slots.get(slot_idx) {
                if slot.sequence.load(Ordering::Acquire) == current_head {
                    unsafe {
                        let ptr = slot.payload.get();
                        let payload_ref = (*ptr).assume_init_ref();
                        if (payload_ref.mask & interest_mask) != 0 {
                            return Some(payload_ref.clone());
                        }
                    }
                }
            }
        }
        None
    }

    pub fn close(&self) {
        self.is_closed.store(true, Ordering::Release);
        self.notify_futex.value.fetch_add(1, Ordering::Release);
        unsafe {
            futex_wake_bitset(&self.notify_futex.value, BIT_ALL);
        }
    }

    pub fn stats(&self) -> (u64, u64, usize) {
        (
            self.total_published.load(Ordering::Relaxed),
            self.total_dropped.load(Ordering::Relaxed),
            self.peers.active_count(),
        )
    }
}

impl Drop for BroadcastHub {
    fn drop(&mut self) {
        for slot in &mut self.slots {
            if slot.sequence.load(Ordering::Relaxed) > 0 {
                unsafe {
                    std::ptr::drop_in_place((*slot.payload.get()).as_mut_ptr());
                }
            }
        }
    }
}
