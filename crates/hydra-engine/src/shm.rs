//! POSIX Shared Memory (/dev/shm) Zero-Copy Ring Buffer
//! Lock-free circular slot architecture with atomic sequence pointers.

use std::fs::{File, OpenOptions};
use std::io::{self, ErrorKind};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use memmap2::{MmapMut, MmapOptions};

pub const HYDRA_MAGIC: u32 = 0x48594452; // "HYDR"
pub const HYDRA_VERSION: u32 = 1;
pub const DEFAULT_SLOTS: usize = 16;

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct ShmHeader {
    pub magic: u32,
    pub version: u32,
    pub width: u32,
    pub height: u32,
    pub format: u32, // 1: RGB24, 2: BGR24, 3: NV12, 4: RGBA32
    pub slot_count: u32,
    pub slot_size: u32,
    pub write_sequence: u64,
}

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct SlotHeader {
    pub sequence: u64,
    pub timestamp_us: u64,
    pub frame_index: u64,
    pub payload_size: u32,
    pub flags: u32,
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

        // Initialize Header
        let header = ShmHeader {
            magic: HYDRA_MAGIC,
            version: HYDRA_VERSION,
            width,
            height,
            format,
            slot_count: slot_count as u32,
            slot_size: slot_size as u32,
            write_sequence: 0,
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

        let slot_header = SlotHeader {
            sequence: seq,
            timestamp_us,
            frame_index: seq,
            payload_size: data.len() as u32,
            flags: 0,
        };

        unsafe {
            let base_ptr = self.mmap.as_mut_ptr();
            let slot_hdr_ptr = base_ptr.add(slot_offset) as *mut SlotHeader;
            std::ptr::write_volatile(slot_hdr_ptr, slot_header);

            let data_ptr = base_ptr.add(slot_offset + std::mem::size_of::<SlotHeader>());
            std::ptr::copy_nonoverlapping(data.as_ptr(), data_ptr, data.len());

            let global_hdr_ptr = base_ptr as *mut ShmHeader;
            let write_seq_atomic = &*(&((*global_hdr_ptr).write_sequence) as *const u64 as *const AtomicU64);
            write_seq_atomic.store(seq, Ordering::Release);
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
}
