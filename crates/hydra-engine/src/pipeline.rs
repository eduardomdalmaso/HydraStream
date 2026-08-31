//! HydraStream Video Pipeline Engine

use std::collections::HashMap;
use std::io;
use std::time::Instant;
use crate::governor::FpsGovernor;
use crate::shm::ShmWriter;

pub struct ConsumerEngine {
    pub analytic_type: String,
    pub governor: FpsGovernor,
    pub format: String,
}

pub struct StreamPipeline {
    pub stream_id: String,
    pub width: u32,
    pub height: u32,
    pub writer: ShmWriter,
    pub consumers: HashMap<String, ConsumerEngine>,
    pub frame_count: u64,
    pub start_time: Instant,
    pub last_frame_time: Instant,
}

impl StreamPipeline {
    pub fn new(stream_id: &str, width: u32, height: u32, format: u32, slot_count: usize) -> io::Result<Self> {
        let writer = ShmWriter::create(stream_id, width, height, format, slot_count)?;
        let now = Instant::now();

        Ok(Self {
            stream_id: stream_id.to_string(),
            width,
            height,
            writer,
            consumers: HashMap::new(),
            frame_count: 0,
            start_time: now,
            last_frame_time: now,
        })
    }

    pub fn register_consumer(&mut self, analytic_type: &str, target_fps: f64, format: &str) {
        let engine = ConsumerEngine {
            analytic_type: analytic_type.to_string(),
            governor: FpsGovernor::new(target_fps),
            format: format.to_string(),
        };
        self.consumers.insert(analytic_type.to_string(), engine);
    }

    pub fn ingest_frame(&mut self, timestamp_us: u64, frame_data: &[u8]) -> io::Result<u64> {
        self.frame_count += 1;
        self.last_frame_time = Instant::now();
        self.writer.write_frame(timestamp_us, frame_data)
    }

    pub fn generate_synthetic_frame(&self, frame_idx: u64) -> Vec<u8> {
        let len = (self.width * self.height * 3) as usize;
        let mut buf = vec![0u8; len];
        let color_shift = (frame_idx % 255) as u8;

        for (i, byte) in buf.iter_mut().enumerate() {
            *byte = (i as u8).wrapping_add(color_shift);
        }
        buf
    }

    pub fn current_fps(&self) -> f64 {
        let elapsed = self.start_time.elapsed().as_secs_f64();
        if elapsed > 0.0 {
            (self.frame_count as f64) / elapsed
        } else {
            0.0
        }
    }
}
