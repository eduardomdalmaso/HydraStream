//! Multi-Consumer Broadcast Hub with Selective Futex Bitset Waking

use std::cell::UnsafeCell;
use std::mem::MaybeUninit;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::Duration;

use super::futex::*;
use super::payload::*;
use super::peer_list::LockFreePeerList;

#[repr(C, align(64))]
pub struct RawBroadcastSlot {
    pub sequence: AtomicU64,
    pub payload: UnsafeCell<MaybeUninit<FramePayload>>,
}

impl RawBroadcastSlot {
    pub fn new() -> Self {
        Self {
            sequence: AtomicU64::new(0),
            payload: UnsafeCell::new(MaybeUninit::uninit()),
        }
    }
}

pub struct BroadcastHub {
    pub capacity: usize,
    pub slots: Vec<RawBroadcastSlot>,
    pub head_seq: CacheAlignedAtomicU64,
    pub notify_futex: CacheAlignedAtomicU32,
    pub peers: LockFreePeerList,
    pub total_published: AtomicU64,
    pub total_dropped: AtomicU64,
    pub is_closed: AtomicBool,
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

        unsafe {
            let ptr = slot.payload.get();
            if slot.sequence.load(Ordering::Relaxed) > 0 {
                std::ptr::drop_in_place((*ptr).as_mut_ptr());
            }
            (*ptr).write(frame);
        }

        slot.sequence.store(seq, Ordering::Release);
        self.head_seq.value.store(seq, Ordering::Release);
        self.total_published.fetch_add(1, Ordering::Relaxed);

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
