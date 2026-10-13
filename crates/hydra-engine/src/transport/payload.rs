//! Frame Payload & Zero-Overhead Memory Layout

use std::sync::Arc;

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
    pub mask: u32,
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
