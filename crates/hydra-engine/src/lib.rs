pub mod shm;
pub mod governor;
pub mod pipeline;
pub mod transport;
pub mod ffi;

pub use shm::{ShmWriter, ShmReader, ShmHeader, SlotHeader};
pub use governor::FpsGovernor;
pub use pipeline::StreamPipeline;
pub use transport::{BroadcastHub, FramePayload, FrameStorage, CacheAlignedAtomicU64, CacheAlignedAtomicU32};

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::thread;
    use std::time::Duration;

    #[test]
    fn test_cache_alignment_and_sizes() {
        assert_eq!(std::mem::size_of::<ShmHeader>(), 64);
        assert_eq!(std::mem::align_of::<ShmHeader>(), 64);

        assert_eq!(std::mem::size_of::<SlotHeader>(), 64);
        assert_eq!(std::mem::align_of::<SlotHeader>(), 64);

        assert_eq!(std::mem::align_of::<CacheAlignedAtomicU64>(), 64);
        assert_eq!(std::mem::align_of::<CacheAlignedAtomicU32>(), 64);
    }

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
    fn test_shm_futex_blocking_wait() {
        let stream_id = "test_stream_futex";
        let width = 32;
        let height = 32;
        let format = 1;
        let slots = 4;

        let mut writer = ShmWriter::create(stream_id, width, height, format, slots)
            .expect("Failed to create ShmWriter");

        let stream_id_clone = stream_id.to_string();
        let handle = thread::spawn(move || {
            let mut reader = ShmReader::open(&stream_id_clone)
                .expect("Failed to open ShmReader in background thread");
            let mut buf = Vec::new();
            
            // This blocks using Futex (0% CPU) until the writer writes the frame
            let meta = reader.wait_and_read_frame(&mut buf, Some(Duration::from_secs(2)))
                .expect("Futex wait error")
                .expect("Expected frame meta");
            (meta.sequence, buf)
        });

        // Sleep briefly to ensure reader is blocked on Futex
        thread::sleep(Duration::from_millis(50));

        let frame_data = vec![99u8; 32 * 32 * 3];
        writer.write_frame(2_000_000, &frame_data).expect("Failed to write frame");

        let (received_seq, received_buf) = handle.join().expect("Reader thread panicked");
        assert_eq!(received_seq, 1);
        assert_eq!(received_buf.len(), 32 * 32 * 3);
        assert_eq!(received_buf[0], 99);
    }

    #[test]
    fn test_broadcast_hub_multi_consumer_concurrency() {
        let hub = Arc::new(BroadcastHub::new(16));
        let num_consumers = 4;
        let frames_to_publish = 50;

        let mut handles = Vec::new();

        for _ in 0..num_consumers {
            let hub_clone = Arc::clone(&hub);
            let handle = thread::spawn(move || {
                let mut last_seq = 0;
                let mut count = 0;
                while count < frames_to_publish {
                    if let Some(frame) = hub_clone.wait_and_read(last_seq) {
                        assert!(frame.frame_id > last_seq);
                        last_seq = frame.frame_id;
                        count += 1;
                    } else {
                        break;
                    }
                }
                count
            });
            handles.push(handle);
        }

        // Producer thread
        for i in 1..=frames_to_publish {
            let payload = FramePayload::new_host(
                i as u64,
                (i * 33_333) as u64,
                640,
                480,
                1,
                vec![i as u8; 640 * 480 * 3],
            );
            hub.publish(payload);
            thread::sleep(Duration::from_millis(1));
        }

        for handle in handles {
            let frames_received = handle.join().expect("Consumer panicked");
            assert_eq!(frames_received, frames_to_publish);
        }

        let (published, _dropped, _active) = hub.stats();
        assert_eq!(published, frames_to_publish as u64);
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
