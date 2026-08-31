pub mod shm;
pub mod governor;
pub mod pipeline;
pub mod ffi;

pub use shm::{ShmWriter, ShmReader, ShmHeader, SlotHeader};
pub use governor::FpsGovernor;
pub use pipeline::StreamPipeline;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_shm_write_read_roundtrip() {
        let stream_id = "test_stream_01";
        let width = 64;
        let height = 64;
        let format = 1; // RGB24
        let slots = 4;

        let mut writer = ShmWriter::create(stream_id, width, height, format, slots)
            .expect("Failed to create ShmWriter");

        let frame_size = (width * height * 3) as usize;
        let test_payload = vec![42u8; frame_size];

        let seq = writer.write_frame(1_000_000, &test_payload)
            .expect("Failed to write frame");
        assert_eq!(seq, 1);

        let mut reader = ShmReader::open(stream_id)
            .expect("Failed to open ShmReader");

        let mut read_buf = Vec::new();
        let meta = reader.read_latest_frame(&mut read_buf)
            .expect("Failed to read latest frame")
            .expect("Expected frame meta");

        assert_eq!(meta.sequence, 1);
        assert_eq!(meta.timestamp_us, 1_000_000);
        assert_eq!(read_buf.len(), frame_size);
        assert_eq!(read_buf[0], 42);
        assert_eq!(read_buf[frame_size - 1], 42);
    }

    #[test]
    fn test_fps_governor_sampling() {
        let mut gov = FpsGovernor::new(2.0); // 2 FPS => 500,000 us interval

        assert!(gov.should_dispatch(0));
        assert!(!gov.should_dispatch(100_000));
        assert!(!gov.should_dispatch(400_000));
        assert!(gov.should_dispatch(500_001));
        assert!(!gov.should_dispatch(600_000));
        assert!(gov.should_dispatch(1_000_002));

        let (passed, dropped) = gov.stats();
        assert_eq!(passed, 3);
        assert_eq!(dropped, 3);
    }
}
