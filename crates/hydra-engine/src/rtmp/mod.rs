//! Native Tokio RTMP Push Ingestion Server for HydraStream Engine
//! High-throughput zero-copy RTMP ingest directly into POSIX SHM and WSP broadcast.

pub mod handshake;
pub mod amf;
pub mod chunk;
pub mod video;
pub mod server;

pub use handshake::rtmp_handshake;
pub use chunk::{RtmpMessage, read_rtmp_message, write_rtmp_message};
pub use video::{convert_avcc_to_annexb, extract_sps_pps_from_avcc, CODEC_AVC_H264, CODEC_HEVC_H265};
pub use server::RtmpServer;
