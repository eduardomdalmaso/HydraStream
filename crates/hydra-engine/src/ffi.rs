//! C-ABI Foreign Function Interface (FFI) for Go and Python interop

use std::ffi::CStr;
use std::os::raw::{c_char, c_int, c_uchar};
use crate::shm::{ShmReader, ShmWriter, SlotHeader};

#[no_mangle]
pub unsafe extern "C" fn hydra_shm_create(
    stream_id: *const c_char,
    width: u32,
    height: u32,
    format: u32,
    slot_count: usize,
) -> *mut ShmWriter {
    if stream_id.is_null() {
        return std::ptr::null_mut();
    }
    let c_str = CStr::from_ptr(stream_id);
    let id = match c_str.to_str() {
        Ok(s) => s,
        Err(_) => return std::ptr::null_mut(),
    };

    match ShmWriter::create(id, width, height, format, slot_count) {
        Ok(writer) => Box::into_raw(Box::new(writer)),
        Err(_) => std::ptr::null_mut(),
    }
}

#[no_mangle]
pub unsafe extern "C" fn hydra_shm_write(
    handle: *mut ShmWriter,
    timestamp_us: u64,
    data: *const c_uchar,
    len: usize,
) -> i64 {
    if handle.is_null() || data.is_null() {
        return -1;
    }
    let writer = &mut *handle;
    let slice = std::slice::from_raw_parts(data, len);

    match writer.write_frame(timestamp_us, slice) {
        Ok(seq) => seq as i64,
        Err(_) => -1,
    }
}

#[no_mangle]
pub unsafe extern "C" fn hydra_shm_destroy(handle: *mut ShmWriter) {
    if !handle.is_null() {
        let _ = Box::from_raw(handle);
    }
}

#[no_mangle]
pub unsafe extern "C" fn hydra_shm_reader_open(stream_id: *const c_char) -> *mut ShmReader {
    if stream_id.is_null() {
        return std::ptr::null_mut();
    }
    let c_str = CStr::from_ptr(stream_id);
    let id = match c_str.to_str() {
        Ok(s) => s,
        Err(_) => return std::ptr::null_mut(),
    };

    match ShmReader::open(id) {
        Ok(reader) => Box::into_raw(Box::new(reader)),
        Err(_) => std::ptr::null_mut(),
    }
}

#[no_mangle]
pub unsafe extern "C" fn hydra_shm_reader_close(handle: *mut ShmReader) {
    if !handle.is_null() {
        let _ = Box::from_raw(handle);
    }
}

#[no_mangle]
pub unsafe extern "C" fn hydra_shm_read_latest(
    handle: *mut ShmReader,
    out_buf: *mut c_uchar,
    max_len: usize,
    out_meta: *mut SlotHeader,
) -> c_int {
    if handle.is_null() || out_buf.is_null() {
        return -1;
    }
    let reader = &mut *handle;
    let mut temp = Vec::new();

    match reader.read_latest_frame(&mut temp) {
        Ok(Some(meta)) => {
            if temp.len() > max_len {
                return -2;
            }
            std::ptr::copy_nonoverlapping(temp.as_ptr(), out_buf, temp.len());
            if !out_meta.is_null() {
                std::ptr::write_volatile(out_meta, meta);
            }
            temp.len() as c_int
        }
        Ok(None) => 0, // No new frame
        Err(_) => -1,
    }
}
