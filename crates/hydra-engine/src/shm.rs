//! POSIX Shared Memory (/dev/shm) Zero-Copy Ring Buffer
//! Lock-free circular slot architecture with atomic sequence pointers,
//! cache-line alignment (64-byte boundary), and OS Futex synchronization.

use std::fs::{File, OpenOptions};
use std::io::{self, ErrorKind};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU32, AtomicU64, Ordering};
use std::time::Duration;
use memmap2::{MmapMut, MmapOptions};

pub const HYDRA_MAGIC: u32 = 0x48594452; // "HYDR"
pub const HYDRA_VERSION: u32 = 1;
pub const DEFAULT_SLOTS: usize = 16;

/// Process-shared Futex primitives for Linux memory-mapped files
#[cfg(target_os = "linux")]
pub(crate) unsafe fn futex_wait_shared(uaddr: *const AtomicU32, val: u32, timeout: Option<Duration>) -> bool {
    let ts = timeout.map(|d| libc::timespec {
        tv_sec: d.as_secs() as libc::time_t,
        tv_nsec: d.subsec_nanos() as libc::c_long,
    });
    let ts_ptr = ts.as_ref().map_or(std::ptr::null(), |t| t as *const libc::timespec);

    let ret = libc::syscall(
        libc::SYS_futex,
        uaddr as *const u32,
        libc::FUTEX_WAIT, // 0 = Process-Shared FUTEX_WAIT
        val,
        ts_ptr,
        std::ptr::null::<u32>(),
        0u32,
    );
    ret == 0
}

#[cfg(target_os = "linux")]
pub(crate) unsafe fn futex_wake_shared(uaddr: *const AtomicU32, count: i32) -> i32 {
    let ret = libc::syscall(
        libc::SYS_futex,
        uaddr as *const u32,
        libc::FUTEX_WAKE, // 1 = Process-Shared FUTEX_WAKE
        count,
        std::ptr::null::<libc::timespec>(),
        std::ptr::null::<u32>(),
        0u32,
    );
    ret as i32
}

#[cfg(not(target_os = "linux"))]
pub(crate) unsafe fn futex_wait_shared(uaddr: *const AtomicU32, val: u32, _timeout: Option<Duration>) -> bool {
    atomic_wait::wait(&*uaddr, val);
    true
}

#[cfg(not(target_os = "linux"))]
pub(crate) unsafe fn futex_wake_shared(uaddr: *const AtomicU32, _count: i32) -> i32 {
    atomic_wait::wake_all(&*uaddr);
    1
}

/// Cache-line aligned SHM Global Header (64 bytes).
/// Isolates write_sequence and notify_seq to prevent False Sharing.
#[repr(C, align(64))]
#[derive(Debug, Clone, Copy)]
pub struct ShmHeader {
    pub magic: u32,
    pub version: u32,
    pub width: u32,
    pub height: u32,
    pub format: u32, // 1: RGB24, 2: BGR24, 3: NV12, 4: RGBA32
    pub slot_count: u32,
    pub slot_size: u32,
    pub notify_seq: u32,     // 32-bit Futex notification word
    pub write_sequence: u64, // 64-bit Monotonic sequence with Release/Acquire ordering
    pub _reserved: [u8; 24], // Padding to strictly match 64-byte cache line
}

/// Cache-line aligned Slot Header (64 bytes).
/// Guarantees that each slot descriptor starts at a 64-byte boundary.
#[repr(C, align(64))]
#[derive(Debug, Clone, Copy)]
pub struct SlotHeader {
    pub sequence: u64,
    pub timestamp_us: u64,
    pub frame_index: u64,
    pub payload_size: u32,
    pub flags: u32,
    pub _reserved: [u8; 32], // Padding to fill 64 bytes
}

#[allow(dead_code)]
pub struct ShmWriter {
    path: PathBuf,
    file: File,
    mmap: MmapMut,
    width: u32,
    height: u32,
    format: u32,
    slot_count: usize,
    slot_size: usize,
    total_size: usize,
    frame_counter: u64,
}

impl ShmWriter {
    pub fn create(stream_id: &str, width: u32, height: u32, format: u32, slot_count: usize) -> io::Result<Self> {
        let slot_count = if slot_count == 0 { DEFAULT_SLOTS } else { slot_count };
        let bytes_per_pixel = match format {
            4 => 4, // RGBA
            _ => 3, // RGB24 / BGR24
        };
        let frame_bytes = (width * height * bytes_per_pixel) as usize;
        let slot_size = std::mem::size_of::<SlotHeader>() + frame_bytes;
        let header_size = std::mem::size_of::<ShmHeader>();
        let total_size = header_size + (slot_count * slot_size);

        let shm_dir = Path::new("/dev/shm");
        let path = if shm_dir.exists() {
            shm_dir.join(format!("hydra_{}", stream_id))
        } else {
            std::env::temp_dir().join(format!("hydra_{}", stream_id))
        };

        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .truncate(true)
            .open(&path)?;

        file.set_len(total_size as u64)?;

        let mut mmap = unsafe { MmapOptions::new().map_mut(&file)? };

        // Initialize Header (aligned to 64 bytes)
        let header = ShmHeader {
            magic: HYDRA_MAGIC,
            version: HYDRA_VERSION,
            width,
            height,
            format,
            slot_count: slot_count as u32,
            slot_size: slot_size as u32,
            notify_seq: 0,
            write_sequence: 0,
            _reserved: [0u8; 24],
        };

        unsafe {
            let ptr = mmap.as_mut_ptr() as *mut ShmHeader;
            std::ptr::write_volatile(ptr, header);
        }

        Ok(Self {
            path,
            file,
            mmap,
            width,
            height,
            format,
            slot_count,
            slot_size,
            total_size,
            frame_counter: 0,
        })
    }

    pub fn write_frame(&mut self, timestamp_us: u64, data: &[u8]) -> io::Result<u64> {
        let expected_frame_size = self.slot_size - std::mem::size_of::<SlotHeader>();
        if data.len() > expected_frame_size {
            return Err(io::Error::new(
                ErrorKind::InvalidInput,
                format!("Frame size {} exceeds slot capacity {}", data.len(), expected_frame_size),
            ));
        }

        self.frame_counter += 1;
        let seq = self.frame_counter;
        let slot_idx = (seq as usize) % self.slot_count;
        let header_size = std::mem::size_of::<ShmHeader>();
        let slot_offset = header_size + (slot_idx * self.slot_size);
        if slot_offset + self.slot_size > self.mmap.len() {
            return Err(io::Error::new(ErrorKind::UnexpectedEof, "Slot offset out of bounds"));
        }

        let slot_header = SlotHeader {
            sequence: seq,
            timestamp_us,
            frame_index: seq,
            payload_size: data.len() as u32,
            flags: 0,
            _reserved: [0u8; 32],
        };

        unsafe {
            let base_ptr = self.mmap.as_mut_ptr();
            let slot_hdr_ptr = base_ptr.add(slot_offset) as *mut SlotHeader;
            std::ptr::write_volatile(slot_hdr_ptr, slot_header);

            let data_ptr = base_ptr.add(slot_offset + std::mem::size_of::<SlotHeader>());
            std::ptr::copy_nonoverlapping(data.as_ptr(), data_ptr, data.len());

            let global_hdr_ptr = base_ptr as *mut ShmHeader;
            
            // Release ordering: Ensures payload data and slot header are committed before write_sequence is updated
            let write_seq_atomic = &*(&((*global_hdr_ptr).write_sequence) as *const u64 as *const AtomicU64);
            write_seq_atomic.store(seq, Ordering::Release);

            // Futex notification: Increment notify_seq and wake sleeping reader processes/threads
            let notify_seq_atomic = &*(&((*global_hdr_ptr).notify_seq) as *const u32 as *const AtomicU32);
            notify_seq_atomic.fetch_add(1, Ordering::Release);
            futex_wake_shared(notify_seq_atomic, i32::MAX);
        }

        Ok(seq)
    }

    pub fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for ShmWriter {
    fn drop(&mut self) {
        let _ = std::fs::remove_file(&self.path);
    }
}

#[allow(dead_code)]
pub struct ShmReader {
    path: PathBuf,
    mmap: memmap2::Mmap,
    header: ShmHeader,
    last_seen_seq: u64,
}

impl ShmReader {
    pub fn open(stream_id: &str) -> io::Result<Self> {
        let shm_dir = Path::new("/dev/shm");
        let path = if shm_dir.exists() {
            shm_dir.join(format!("hydra_{}", stream_id))
        } else {
            std::env::temp_dir().join(format!("hydra_{}", stream_id))
        };

        let file = OpenOptions::new().read(true).open(&path)?;
        let mmap = unsafe { MmapOptions::new().map(&file)? };

        if mmap.len() < std::mem::size_of::<ShmHeader>() {
            return Err(io::Error::new(ErrorKind::UnexpectedEof, "SHM file too small for header"));
        }

        let header = unsafe {
            let ptr = mmap.as_ptr() as *const ShmHeader;
            std::ptr::read_volatile(ptr)
        };

        if header.magic != HYDRA_MAGIC {
            return Err(io::Error::new(ErrorKind::InvalidData, "Invalid HydraStream magic signature"));
        }

        Ok(Self {
            path,
            mmap,
            header,
            last_seen_seq: 0,
        })
    }

    pub fn header(&self) -> &ShmHeader {
        &self.header
    }

    /// Non-blocking read of the latest frame.
    pub fn read_latest_frame(&mut self, out_buffer: &mut Vec<u8>) -> io::Result<Option<SlotHeader>> {
        let global_hdr_ptr = self.mmap.as_ptr() as *const ShmHeader;
        let current_seq = unsafe {
            let write_seq_atomic = &*(&((*global_hdr_ptr).write_sequence) as *const u64 as *const AtomicU64);
            write_seq_atomic.load(Ordering::Acquire)
        };

        if current_seq == 0 || current_seq <= self.last_seen_seq {
            return Ok(None);
        }

        let slot_idx = (current_seq as usize) % (self.header.slot_count as usize);
        let header_size = std::mem::size_of::<ShmHeader>();
        let slot_offset = header_size + (slot_idx * (self.header.slot_size as usize));

        if slot_offset + (self.header.slot_size as usize) > self.mmap.len() {
            return Err(io::Error::new(ErrorKind::UnexpectedEof, "Slot offset out of bounds"));
        }

        let (slot_header, payload_slice) = unsafe {
            let base_ptr = self.mmap.as_ptr();
            let slot_hdr_ptr = base_ptr.add(slot_offset) as *const SlotHeader;
            let hdr = std::ptr::read_volatile(slot_hdr_ptr);

            let data_ptr = base_ptr.add(slot_offset + std::mem::size_of::<SlotHeader>());
            let slice = std::slice::from_raw_parts(data_ptr, hdr.payload_size as usize);
            (hdr, slice)
        };

        out_buffer.clear();
        out_buffer.extend_from_slice(payload_slice);
        self.last_seen_seq = current_seq;

        Ok(Some(slot_header))
    }

    /// Blocking read using OS Futex (0% CPU while waiting for next frame).
    pub fn wait_and_read_frame(&mut self, out_buffer: &mut Vec<u8>, timeout: Option<Duration>) -> io::Result<Option<SlotHeader>> {
        let global_hdr_ptr = self.mmap.as_ptr() as *const ShmHeader;
        let write_seq_atomic = unsafe {
            &*(&((*global_hdr_ptr).write_sequence) as *const u64 as *const AtomicU64)
        };
        let notify_seq_atomic = unsafe {
            &*(&((*global_hdr_ptr).notify_seq) as *const u32 as *const AtomicU32)
        };

        let start = std::time::Instant::now();
        loop {
            let current_seq = write_seq_atomic.load(Ordering::Acquire);
            if current_seq > self.last_seen_seq {
                if let Some(hdr) = self.read_latest_frame(out_buffer)? {
                    return Ok(Some(hdr));
                }
            }

            if let Some(t) = timeout {
                if start.elapsed() >= t {
                    return Ok(None);
                }
            }

            // Sleep in OS kernel via process-shared Futex
            let current_notify = notify_seq_atomic.load(Ordering::Acquire);
            unsafe {
                futex_wait_shared(notify_seq_atomic, current_notify, timeout);
            }
        }
    }
}
