pub mod shm;
pub mod governor;
pub mod pipeline;
pub mod transport;
pub mod sync;
pub mod codec;
pub mod gateway;
pub mod ffi;

pub use shm::{ShmWriter, ShmReader, ShmHeader, SlotHeader};
pub use governor::FpsGovernor;
pub use pipeline::{StreamPipeline, StreamPipelineBuilder};
pub use transport::{
    BroadcastHub, FramePayload, FrameStorage, LockFreePeerList, RcuConfig,
    CacheAlignedAtomicU64, CacheAlignedAtomicU32,
    BIT_WEBSOCKET, BIT_WEBRTC, BIT_ANALYTICS, BIT_ALL,
};
pub use sync::{AtomicSemaphore, HybridMutex, ParkingTable};
pub use codec::{NalParser, NalType, GopTracker, FLAG_KEYFRAME, FLAG_SPS_PPS, FLAG_DELTA_FRAME, FLAG_GPU_SURFACE};
pub use gateway::{AsyncStreamGateway, ClientSession, StreamError};

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
            
            let meta = reader.wait_and_read_frame(&mut buf, Some(Duration::from_secs(2)))
                .expect("Futex wait error")
                .expect("Expected frame meta");
            (meta.sequence, buf)
        });

        thread::sleep(Duration::from_millis(50));

        let frame_data = vec![99u8; 32 * 32 * 3];
        writer.write_frame(2_000_000, &frame_data).expect("Failed to write frame");

        let (received_seq, received_buf) = handle.join().expect("Reader thread panicked");
        assert_eq!(received_seq, 1);
        assert_eq!(received_buf.len(), 32 * 32 * 3);
        assert_eq!(received_buf[0], 99);
    }

    #[test]
    fn test_lock_free_peer_list() {
        let list = LockFreePeerList::new();
        assert_eq!(list.active_count(), 0);

        list.register(101, BIT_WEBSOCKET);
        list.register(102, BIT_WEBRTC);
        list.register(103, BIT_ANALYTICS);
        assert_eq!(list.active_count(), 3);

        list.mark_inactive(102);
        assert_eq!(list.active_count(), 2);

        let mut collected = Vec::new();
        list.for_each(|peer| {
            collected.push(peer.client_id);
        });
        assert!(collected.contains(&101));
        assert!(collected.contains(&103));
        assert!(!collected.contains(&102));
    }

    #[test]
    fn test_rcu_config_hot_swap() {
        #[derive(Debug, PartialEq, Eq)]
        struct CamConfig {
            width: u32,
            fps: u32,
        }

        let rcu = RcuConfig::new(CamConfig { width: 1920, fps: 30 });
        assert_eq!(rcu.load().width, 1920);
        assert_eq!(rcu.load().fps, 30);

        rcu.update(CamConfig { width: 3840, fps: 60 });
        assert_eq!(rcu.load().width, 3840);
        assert_eq!(rcu.load().fps, 60);
    }

    #[test]
    fn test_broadcast_hub_selective_bitset_concurrency() {
        let hub = Arc::new(BroadcastHub::new(16));
        
        let hub_ws = Arc::clone(&hub);
        let handle_ws = thread::spawn(move || {
            let mut received = Vec::new();
            for _ in 0..5 {
                if let Some(frame) = hub_ws.wait_and_read(
                    received.last().copied().unwrap_or(0),
                    BIT_WEBSOCKET,
                    Some(Duration::from_secs(2)),
                ) {
                    received.push(frame.frame_id);
                }
            }
            received
        });

        let hub_webrtc = Arc::clone(&hub);
        let handle_webrtc = thread::spawn(move || {
            let mut received = Vec::new();
            for _ in 0..5 {
                if let Some(frame) = hub_webrtc.wait_and_read(
                    received.last().copied().unwrap_or(0),
                    BIT_WEBRTC,
                    Some(Duration::from_secs(2)),
                ) {
                    received.push(frame.frame_id);
                }
            }
            received
        });

        thread::sleep(Duration::from_millis(50));

        for i in 1..=10 {
            let mask = if i % 2 == 0 { BIT_WEBSOCKET } else { BIT_WEBRTC };
            let payload = FramePayload::new_host(
                i as u64,
                (i * 33_333) as u64,
                640,
                480,
                1,
                mask,
                vec![i as u8; 640 * 480 * 3],
            );
            hub.publish(payload);
            thread::sleep(Duration::from_millis(5));
        }

        let ws_frames = handle_ws.join().expect("WS consumer panicked");
        let webrtc_frames = handle_webrtc.join().expect("WebRTC consumer panicked");

        assert_eq!(ws_frames.len(), 5);
        assert_eq!(webrtc_frames.len(), 5);
        assert_eq!(ws_frames, vec![2, 4, 6, 8, 10]);
        assert_eq!(webrtc_frames, vec![1, 3, 5, 7, 9]);
    }

    #[test]
    fn test_atomic_semaphore_backpressure() {
        let sem = Arc::new(AtomicSemaphore::new(2));
        assert_eq!(sem.available(), 2);

        assert!(sem.acquire(Some(Duration::from_millis(10))));
        assert!(sem.acquire(Some(Duration::from_millis(10))));
        assert_eq!(sem.available(), 0);

        assert!(!sem.acquire(Some(Duration::from_millis(20))));

        let sem_clone = Arc::clone(&sem);
        thread::spawn(move || {
            thread::sleep(Duration::from_millis(30));
            sem_clone.release();
        });

        assert!(sem.acquire(Some(Duration::from_secs(1))));
    }

    #[test]
    fn test_hybrid_mutex_concurrency() {
        let count = Arc::new(HybridMutex::new(0u64));
        let threads: Vec<_> = (0..8)
            .map(|_| {
                let c = Arc::clone(&count);
                thread::spawn(move || {
                    for _ in 0..1_000 {
                        let mut guard = c.lock();
                        *guard += 1;
                    }
                })
            })
            .collect();

        for t in threads {
            t.join().expect("Thread panicked");
        }

        assert_eq!(*count.lock(), 8_000);
    }

    #[test]
    fn test_parking_table_park_unpark() {
        let table = Arc::new(ParkingTable::new());
        let test_addr = 0x12345678usize;

        let table_clone = Arc::clone(&table);
        let handle = thread::spawn(move || {
            table_clone.park(test_addr, Some(Duration::from_secs(2)))
        });

        thread::sleep(Duration::from_millis(50));
        let unparked = table.unpark_all(test_addr);
        assert_eq!(unparked, 1);

        assert!(handle.join().expect("Park thread panicked"));
    }

    #[test]
    fn test_scoped_threads_zero_arc_dispatch() {
        let frame = FramePayload::new_host(
            42,
            1_000_000,
            1920,
            1080,
            1,
            BIT_ALL,
            vec![7u8; 100],
        );

        let mut results = vec![0u8; 4];

        thread::scope(|s| {
            for (idx, slot) in results.iter_mut().enumerate() {
                let frame_ref = &frame;
                s.spawn(move || {
                    assert_eq!(frame_ref.frame_id, 42);
                    *slot = (idx as u8) + 1;
                });
            }
        });

        assert_eq!(results, vec![1, 2, 3, 4]);
    }

    #[test]
    fn test_nal_unit_parsing_and_gop_tracker() {
        let h264_idr = [0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84];
        let h264_sps = [0x00, 0x00, 0x01, 0x67, 0x42, 0x00];
        let h264_p_frame = [0x00, 0x00, 0x01, 0x41, 0x9A];

        assert_eq!(NalParser::parse_nal_type(&h264_idr, false), NalType::H264Idr);
        assert_eq!(NalParser::parse_nal_type(&h264_sps, false), NalType::H264Sps);
        assert_eq!(NalParser::parse_nal_type(&h264_p_frame, false), NalType::H264NonIdr);

        let tracker = GopTracker::new();
        tracker.record_frame(1, FLAG_KEYFRAME);
        assert_eq!(tracker.latest_keyframe_seq(), 1);

        for i in 2..=30 {
            tracker.record_frame(i, FLAG_DELTA_FRAME);
        }
        assert_eq!(tracker.latest_keyframe_seq(), 1);

        tracker.record_frame(31, FLAG_KEYFRAME);
        assert_eq!(tracker.latest_keyframe_seq(), 31);
        assert_eq!(tracker.gop_size(), 30);
    }

    #[tokio::test]
    async fn test_async_stream_gateway_tokio_multiclient() {
        let hub = Arc::new(BroadcastHub::new(16));
        let gateway = Arc::new(AsyncStreamGateway::new(hub, 32));

        // Pre-publish a keyframe
        let keyframe = FramePayload::new_host(
            1,
            1_000_000,
            1920,
            1080,
            1,
            BIT_WEBSOCKET,
            vec![10u8; 100],
        );
        let _ = gateway.dispatch_frame(keyframe, FLAG_KEYFRAME);

        // Client 1 connects and immediately gets initial keyframe
        let (mut session1, initial_kf) = gateway.subscribe(BIT_WEBSOCKET).expect("Subscribe failed");
        assert!(initial_kf.is_some());
        assert_eq!(initial_kf.unwrap().frame_id, 1);
        assert_eq!(gateway.active_clients(), 1);

        // Spawn async receiver task
        let handle = tokio::spawn(async move {
            let mut received = Vec::new();
            for _ in 0..3 {
                if let Ok(frame) = session1.receiver.recv().await {
                    received.push(frame.frame_id);
                }
            }
            received
        });

        // Publish 3 delta frames
        for i in 2..=4 {
            let p_frame = FramePayload::new_host(
                i,
                (i * 33_333) as u64,
                1920,
                1080,
                1,
                BIT_WEBSOCKET,
                vec![i as u8; 100],
            );
            let _ = gateway.dispatch_frame(p_frame, FLAG_DELTA_FRAME);
        }

        let received_ids = handle.await.expect("Tokio task failed");
        assert_eq!(received_ids, vec![2, 3, 4]);

        gateway.shutdown();
    }

    #[test]
    fn test_stream_pipeline_builder() {
        let stream_id = "test_builder_stream";
        let pipeline = StreamPipeline::builder(stream_id)
            .with_resolution(1280, 720)
            .with_format(1)
            .with_slots(8)
            .with_consumer("yolo_analytic", 2.0, "rgb24")
            .with_consumer("web_preview", 15.0, "jpeg")
            .build()
            .expect("Failed to build pipeline via builder");

        assert_eq!(pipeline.width, 1280);
        assert_eq!(pipeline.height, 720);
        assert_eq!(pipeline.consumers.len(), 2);
    }

    #[test]
    fn test_nal_parser_malformed_fuzz_safety() {
        // Fuzz-like safety checks against empty, truncated, and corrupt byte sequences
        assert_eq!(NalParser::parse_nal_type(&[], false), NalType::Unknown);
        assert_eq!(NalParser::parse_nal_type(&[0], false), NalType::Unknown);
        assert_eq!(NalParser::parse_nal_type(&[0, 0], false), NalType::Unknown);
        assert_eq!(NalParser::parse_nal_type(&[0, 0, 1], false), NalType::Unknown);
        assert_eq!(NalParser::parse_nal_type(&[0, 0, 0, 1], false), NalType::Unknown);
        assert_eq!(NalParser::parse_nal_type(&[0xFF, 0xEE, 0xDD, 0xCC], false), NalType::Unknown);

        // Valid with trailing corrupt bytes
        let mut malformed_stream = vec![0, 0, 1, 0x65]; // IDR
        malformed_stream.extend_from_slice(&[0xDE, 0xAD, 0xBE, 0xEF]);
        assert_eq!(NalParser::parse_nal_type(&malformed_stream, false), NalType::H264Idr);
        assert_eq!(NalParser::classify_frame_flags(&malformed_stream, false), FLAG_KEYFRAME | FLAG_SPS_PPS);
    }

    #[test]
    fn test_fps_governor_sampling() {
        let mut gov = FpsGovernor::new(2.0);

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
