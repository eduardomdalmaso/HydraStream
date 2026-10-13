//! WSP (WebSocket Stream Protocol) Binary Enveloping
//! High-efficiency binary packing for direct ingestion by W3C WebCodecs VideoDecoder.
//!
//! Packet Layout (9-byte header + Annex-B NALUs):
//! +----------------+-------------------------------+--------------------------+
//! | Frame Type (1B)| Timestamp Microssegundos (8B) | NALU Payload (Annex-B)   |
//! | 0x01 (Keyframe)| Big-Endian uint64             | 00 00 00 01 [SPS/PPS/IDR]|
//! | 0x02 (Delta)   |                               | 00 00 00 01 [Slice P/B]  |
//! +----------------+-------------------------------+--------------------------+

pub const WSP_FRAME_KEYFRAME: u8 = 0x01;
pub const WSP_FRAME_DELTA: u8    = 0x02;
pub const WSP_HEADER_LEN: usize  = 9;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WspFrameRef<'a> {
    pub is_keyframe: bool,
    pub timestamp_us: u64,
    pub payload: &'a [u8],
}

/// Packs raw NAL unit into WSP binary frame with zero reallocations.
#[inline(always)]
pub fn pack_wsp_frame(is_keyframe: bool, timestamp_us: u64, payload: &[u8]) -> Vec<u8> {
    let mut buf = Vec::with_capacity(WSP_HEADER_LEN + payload.len());
    let frame_type = if is_keyframe { WSP_FRAME_KEYFRAME } else { WSP_FRAME_DELTA };
    buf.push(frame_type);
    buf.extend_from_slice(&timestamp_us.to_be_bytes());
    buf.extend_from_slice(payload);
    buf
}

/// Packs into an existing buffer to eliminate memory allocations in hot loop.
#[inline(always)]
pub fn pack_wsp_frame_into(is_keyframe: bool, timestamp_us: u64, payload: &[u8], out: &mut Vec<u8>) {
    out.clear();
    out.reserve(WSP_HEADER_LEN + payload.len());
    let frame_type = if is_keyframe { WSP_FRAME_KEYFRAME } else { WSP_FRAME_DELTA };
    out.push(frame_type);
    out.extend_from_slice(&timestamp_us.to_be_bytes());
    out.extend_from_slice(payload);
}

/// Unpacks a WSP binary frame slice without allocations.
#[inline(always)]
pub fn unpack_wsp_frame(data: &[u8]) -> Option<WspFrameRef<'_>> {
    if data.len() < WSP_HEADER_LEN {
        return None;
    }

    let frame_type = data[0];
    let is_keyframe = match frame_type {
        WSP_FRAME_KEYFRAME => true,
        WSP_FRAME_DELTA => false,
        _ => return None,
    };

    let mut ts_bytes = [0u8; 8];
    ts_bytes.copy_from_slice(&data[1..9]);
    let timestamp_us = u64::from_be_bytes(ts_bytes);
    let payload = &data[9..];

    Some(WspFrameRef {
        is_keyframe,
        timestamp_us,
        payload,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_wsp_pack_unpack_roundtrip() {
        let nalu = [0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f];
        let ts = 1712345678901234;

        let packed = pack_wsp_frame(true, ts, &nalu);
        assert_eq!(packed.len(), WSP_HEADER_LEN + nalu.len());
        assert_eq!(packed[0], WSP_FRAME_KEYFRAME);

        let unpacked = unpack_wsp_frame(&packed).expect("failed to unpack");
        assert!(unpacked.is_keyframe);
        assert_eq!(unpacked.timestamp_us, ts);
        assert_eq!(unpacked.payload, &nalu);
    }

    #[test]
    fn test_wsp_delta_frame() {
        let nalu = [0x00, 0x00, 0x00, 0x01, 0x41, 0x9a];
        let ts = 999999;

        let packed = pack_wsp_frame(false, ts, &nalu);
        assert_eq!(packed[0], WSP_FRAME_DELTA);

        let unpacked = unpack_wsp_frame(&packed).expect("failed to unpack");
        assert!(!unpacked.is_keyframe);
        assert_eq!(unpacked.timestamp_us, ts);
        assert_eq!(unpacked.payload, &nalu);
    }

    #[test]
    fn test_wsp_invalid_packets() {
        assert!(unpack_wsp_frame(&[]).is_none());
        assert!(unpack_wsp_frame(&[0x01, 0x02, 0x03]).is_none());
        // Invalid type byte
        let invalid_type = [0x09, 0, 0, 0, 0, 0, 0, 0, 1, 0xaa];
        assert!(unpack_wsp_frame(&invalid_type).is_none());
    }
}
