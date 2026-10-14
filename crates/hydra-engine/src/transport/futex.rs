//! Bitset Masks, Cache-Line Aligned Atomics & Futex Bitset Operations

use std::sync::atomic::{AtomicU32, AtomicU64};
use std::time::Duration;

pub const BIT_WEBSOCKET: u32 = 0x00000001;
pub const BIT_WEBRTC: u32    = 0x00000002;
pub const BIT_ANALYTICS: u32 = 0x00000004;
pub const BIT_ALL: u32       = 0xFFFFFFFF;

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

#[cfg(not(target_os = "linux"))]
pub unsafe fn futex_wake_bitset(_uaddr: *const AtomicU32, _bitmask: u32) -> i32 {
    atomic_wait::wake_all(&*_uaddr);
    0
}

#[cfg(target_os = "linux")]
pub unsafe fn futex_wake_one(uaddr: *const AtomicU32) -> i32 {
    libc::syscall(
        libc::SYS_futex,
        uaddr as *const u32,
        libc::FUTEX_WAKE,
        1i32,
        std::ptr::null::<libc::timespec>(),
        std::ptr::null::<u32>(),
        0u32,
    ) as i32
}

#[cfg(not(target_os = "linux"))]
pub unsafe fn futex_wake_one(uaddr: *const AtomicU32) -> i32 {
    atomic_wait::wake_one(&*uaddr);
    0
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
pub unsafe fn futex_wait_bitset(uaddr: *const AtomicU32, val: u32, _bitmask: u32, _timeout: Option<Duration>) -> bool {
    atomic_wait::wait(&*uaddr, val);
    true
}

