//! RTMP Video FLV packet decoding, AVCC to Annex-B conversion and SPS/PPS extraction

use std::time::Instant;

use crate::codec::{FLAG_DELTA_FRAME, FLAG_KEYFRAME, FLAG_SPS_PPS};
use crate::transport::{FramePayload, BIT_ALL};

use super::chunk::RtmpMessage;
use super::server::RtmpSessionState;

pub const CODEC_AVC_H264: u8 = 7;
pub const CODEC_HEVC_H265: u8 = 12;

/// Convert AVCC (4-byte length prefix) to Annex-B (0x00 00 00 01 start code)
pub fn convert_avcc_to_annexb(avcc: &[u8], out: &mut Vec<u8>) {
    let mut cursor = 0;
    while cursor + 4 <= avcc.len() {
        let nalu_len = u32::from_be_bytes(avcc[cursor..cursor + 4].try_into().unwrap()) as usize;
        cursor += 4;
        if cursor + nalu_len > avcc.len() {
            break;
        }

        out.extend_from_slice(&[0x00, 0x00, 0x00, 0x01]);
        out.extend_from_slice(&avcc[cursor..cursor + nalu_len]);
        cursor += nalu_len;
    }
}

/// Extract SPS and PPS NALUs from AVCDecoderConfigurationRecord and format as Annex-B
pub fn extract_sps_pps_from_avcc(record: &[u8]) -> Option<Vec<u8>> {
    if record.len() < 7 {
        return None;
    }

    let mut out = Vec::new();
    let num_sps = (record[5] & 0x1F) as usize;
    let mut cursor = 6;

    for _ in 0..num_sps {
        if cursor + 2 > record.len() {
            return None;
        }
        let sps_len = u16::from_be_bytes(record[cursor..cursor + 2].try_into().unwrap()) as usize;
        cursor += 2;
        if cursor + sps_len > record.len() {
            return None;
        }
        out.extend_from_slice(&[0x00, 0x00, 0x00, 0x01]);
        out.extend_from_slice(&record[cursor..cursor + sps_len]);
        cursor += sps_len;
    }

    if cursor < record.len() {
        let num_pps = record[cursor] as usize;
        cursor += 1;
        for _ in 0..num_pps {
            if cursor + 2 > record.len() {
                break;
            }
            let pps_len = u16::from_be_bytes(record[cursor..cursor + 2].try_into().unwrap()) as usize;
            cursor += 2;
            if cursor + pps_len > record.len() {
                break;
            }
            out.extend_from_slice(&[0x00, 0x00, 0x00, 0x01]);
            out.extend_from_slice(&record[cursor..cursor + pps_len]);
            cursor += pps_len;
        }
    }

    if out.is_empty() {
        None
    } else {
        Some(out)
    }
}

pub fn handle_video_message(
    state: &mut RtmpSessionState,
    msg: &RtmpMessage,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    if msg.payload.len() < 5 {
        return Ok(());
    }

    let header_byte = msg.payload[0];
    let frame_type = (header_byte >> 4) & 0x0F;
    let codec_id = header_byte & 0x0F;

    if codec_id != CODEC_AVC_H264 && codec_id != CODEC_HEVC_H265 {
        return Ok(());
    }

    let avc_packet_type = msg.payload[1];
    let raw_payload = &msg.payload[5..];

    if avc_packet_type == 0 {
        if let Some(annexb_seq) = extract_sps_pps_from_avcc(raw_payload) {
            state.cached_seq_header = Some(annexb_seq);
        }
        return Ok(());
    }

    if avc_packet_type == 1 {
        let is_keyframe = frame_type == 1;
        let mut annexb_frame = Vec::with_capacity(raw_payload.len() + 64);

        if is_keyframe {
            if let Some(ref seq) = state.cached_seq_header {
                annexb_frame.extend_from_slice(seq);
            }
        }

        convert_avcc_to_annexb(raw_payload, &mut annexb_frame);

        if annexb_frame.is_empty() {
            return Ok(());
        }

        state.frame_seq += 1;
        let now_us = (Instant::now().elapsed().as_micros()) as u64;

        if let Some(ref mut writer) = state.shm_writer {
            let _ = writer.write_frame(now_us, &annexb_frame);
        }

        if let Some(ref gateway) = state.gateway {
            let flags = if is_keyframe {
                FLAG_KEYFRAME | FLAG_SPS_PPS
            } else {
                FLAG_DELTA_FRAME
            };

            let payload = FramePayload::new_host(
                state.frame_seq,
                now_us,
                1920,
                1080,
                flags,
                BIT_ALL,
                annexb_frame,
            );
            let _ = gateway.dispatch_frame(payload, flags);
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_avcc_to_annexb_conversion() {
        let avcc_data = vec![
            0x00, 0x00, 0x00, 0x04, 0x65, 0x01, 0x02, 0x03,
            0x00, 0x00, 0x00, 0x03, 0x41, 0x99, 0x88,
        ];

        let mut annexb = Vec::new();
        convert_avcc_to_annexb(&avcc_data, &mut annexb);

        let expected = vec![
            0x00, 0x00, 0x00, 0x01, 0x65, 0x01, 0x02, 0x03,
            0x00, 0x00, 0x00, 0x01, 0x41, 0x99, 0x88,
        ];
        assert_eq!(annexb, expected);
    }

    #[test]
    fn test_extract_sps_pps_from_avcc() {
        let record = vec![
            0x01, 0x64, 0x00, 0x1F, 0xFF,
            0xE1, // 1 SPS
            0x00, 0x04, 0x67, 0x42, 0x00, 0x1E, // SPS len=4
            0x01, // 1 PPS
            0x00, 0x02, 0x68, 0xCE, // PPS len=2
        ];

        let extracted = extract_sps_pps_from_avcc(&record).expect("Should extract SPS and PPS");
        let expected = vec![
            0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1E,
            0x00, 0x00, 0x00, 0x01, 0x68, 0xCE,
        ];
        assert_eq!(extracted, expected);
    }
}
