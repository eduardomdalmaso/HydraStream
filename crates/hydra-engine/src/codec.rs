//! Video Codec Analysis, NAL Unit Parsing, and GOP (Group of Pictures) Keyframe Tracker
//! Hardened against malformed streams, zero-byte slices, and boundary overruns (Effective Rust Item 18 & 30).

use std::sync::atomic::{AtomicU64, AtomicU32, Ordering};

pub const FLAG_KEYFRAME: u32    = 0x00000001; // IDR / I-Frame (Independent Decode Refresh)
pub const FLAG_SPS_PPS: u32     = 0x00000002; // Sequence / Picture Parameter Set
pub const FLAG_DELTA_FRAME: u32 = 0x00000004; // P-Frame / B-Frame
pub const FLAG_GPU_SURFACE: u32 = 0x00000008; // Frame resides in CUDA VRAM

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NalType {
    H264Idr,
    H264Sps,
    H264Pps,
    H264NonIdr,
    H265Idr,
    H265VpsSpsPps,
    H265NonIdr,
    Unknown,
}

/// Hardened NAL Parser to identify keyframe boundaries with zero panics
pub struct NalParser;

impl NalParser {
    /// Detects H.264 / H.265 NAL unit type from raw Annex-B byte stream safely
    pub fn parse_nal_type(data: &[u8], is_hevc: bool) -> NalType {
        let payload = Self::find_nal_start(data);
        if payload.is_empty() {
            return NalType::Unknown;
        }

        let first_byte = match payload.first() {
            Some(&b) => b,
            None => return NalType::Unknown,
        };

        if !is_hevc {
            // H.264: NAL type is lower 5 bits of first byte
            let nal_type = first_byte & 0x1F;
            match nal_type {
                5 => NalType::H264Idr,
                7 => NalType::H264Sps,
                8 => NalType::H264Pps,
                1 => NalType::H264NonIdr,
                _ => NalType::Unknown,
            }
        } else {
            // H.265: NAL type is (first_byte >> 1) & 0x3F
            let nal_type = (first_byte >> 1) & 0x3F;
            match nal_type {
                19 | 20 => NalType::H265Idr,
                32 | 33 | 34 => NalType::H265VpsSpsPps,
                1 | 2 => NalType::H265NonIdr,
                _ => NalType::Unknown,
            }
        }
    }

    /// Finds start code (0x000001 or 0x00000001) using panic-free slice operations
    fn find_nal_start(data: &[u8]) -> &[u8] {
        if data.len() < 3 {
            return &[];
        }

        // Check 3-byte start code 0x000001
        if data.get(0) == Some(&0) && data.get(1) == Some(&0) && data.get(2) == Some(&1) {
            return data.get(3..).unwrap_or(&[]);
        }

        // Check 4-byte start code 0x00000001
        if data.len() >= 4
            && data.get(0) == Some(&0)
            && data.get(1) == Some(&0)
            && data.get(2) == Some(&0)
            && data.get(3) == Some(&1)
        {
            return data.get(4..).unwrap_or(&[]);
        }

        data
    }

    /// Classifies frame and returns slot header flags
    pub fn classify_frame_flags(data: &[u8], is_hevc: bool) -> u32 {
        match Self::parse_nal_type(data, is_hevc) {
            NalType::H264Idr | NalType::H265Idr => FLAG_KEYFRAME | FLAG_SPS_PPS,
            NalType::H264Sps | NalType::H264Pps | NalType::H265VpsSpsPps => FLAG_SPS_PPS,
            NalType::H264NonIdr | NalType::H265NonIdr => FLAG_DELTA_FRAME,
            NalType::Unknown => 0,
        }
    }
}

/// Tracks GOP (Group of Pictures) boundaries and the sequence ID of the latest Keyframe.
pub struct GopTracker {
    latest_keyframe_seq: AtomicU64,
    current_gop_size: AtomicU32,
    last_gop_size: AtomicU32,
}

impl GopTracker {
    pub const fn new() -> Self {
        Self {
            latest_keyframe_seq: AtomicU64::new(0),
            current_gop_size: AtomicU32::new(0),
            last_gop_size: AtomicU32::new(30),
        }
    }

    /// Records a new frame sequence and its flags
    pub fn record_frame(&self, seq: u64, flags: u32) {
        if (flags & FLAG_KEYFRAME) != 0 {
            let prev_gop = self.current_gop_size.swap(1, Ordering::Relaxed);
            if prev_gop > 0 {
                self.last_gop_size.store(prev_gop, Ordering::Relaxed);
            }
            self.latest_keyframe_seq.store(seq, Ordering::Release);
        } else {
            self.current_gop_size.fetch_add(1, Ordering::Relaxed);
        }
    }

    /// Returns the sequence ID of the latest Keyframe (I-Frame)
    pub fn latest_keyframe_seq(&self) -> u64 {
        self.latest_keyframe_seq.load(Ordering::Acquire)
    }

    /// Returns the detected GOP size (distance between I-Frames)
    pub fn gop_size(&self) -> u32 {
        self.last_gop_size.load(Ordering::Relaxed)
    }
}
