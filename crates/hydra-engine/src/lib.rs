pub mod shm;
pub mod governor;
pub mod pipeline;
pub mod transport;
pub mod sync;
pub mod codec;
pub mod gateway;
pub mod actor;
pub mod coroutine;
pub mod resilience;
pub mod decorator;
pub mod isolated;
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
pub use gateway::{
    AsyncStreamGateway, ClientSession, StreamError,
    LocalSessionCache, GatewayRuntimeBuilder, spawn_blocking_shm_bridge,
};
pub use actor::{GopActorHandle, GopStats};
pub use coroutine::{SymmetricCoroutine, CoroutineYield, NalClassifierCoroutine, FpsFilterCoroutine};
pub use resilience::{StreamCircuitBreaker, CircuitState};
pub use decorator::{TelemetryFuture, TelemetryExt, FutureMetricsSink};
pub use isolated::{SyncGatewayBridge, SyncReceiver};

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

    #[tokio::test]
    async fn test_async_broadcast_lagged_client_backpressure() {
        let hub = Arc::new(BroadcastHub::new(16));
        // Small broadcast capacity of 4 to trigger Lagged backpressure easily
        let gateway = Arc::new(AsyncStreamGateway::new(hub, 4));

        let (mut session, _) = gateway.subscribe(BIT_WEBSOCKET).expect("Subscribe failed");

        // Producer rapidly publishes 20 frames
        for i in 1..=20 {
            let frame = FramePayload::new_host(
                i,
                (i * 33_333) as u64,
                640,
                480,
                1,
                BIT_WEBSOCKET,
                vec![i as u8; 50],
            );
            let _ = gateway.dispatch_frame(frame, FLAG_DELTA_FRAME);
        }

        // Slow receiver reads after queue overflow: recovers automatically without crashing
        let latest = tokio::time::timeout(Duration::from_millis(200), session.recv_frame())
            .await
            .expect("Timeout waiting for frame")
            .expect("Failed to receive latest frame");

        // The received frame is one of the newest (e.g. >= 16), recovering from lagged state
        assert!(latest.frame_id >= 16);
    }

    #[tokio::test]
    async fn test_async_cancellation_safety_on_disconnect() {
        let hub = Arc::new(BroadcastHub::new(16));
        let gateway = Arc::new(AsyncStreamGateway::new(hub, 16));

        assert_eq!(gateway.active_clients(), 0);

        // Spawn client task and abort it abruptly
        let gateway_clone = Arc::clone(&gateway);
        let task = tokio::spawn(async move {
            let (_session, _) = gateway_clone.subscribe(BIT_WEBSOCKET).expect("Subscribe failed");
            assert_eq!(gateway_clone.active_clients(), 1);
            // Simulate waiting forever until cancelled
            tokio::time::sleep(Duration::from_secs(60)).await;
        });

        tokio::time::sleep(Duration::from_millis(30)).await;
        assert_eq!(gateway.active_clients(), 1);

        // Abort task
        task.abort();
        let _ = task.await;

        // Cancellation Safety: Dropping the ClientSession on abort decrements active_clients
        assert_eq!(gateway.active_clients(), 0);
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn test_async_multithread_high_concurrency_stress() {
        let hub = Arc::new(BroadcastHub::new(32));
        let gateway = Arc::new(AsyncStreamGateway::new(hub, 64));

        let num_subscribers = 8;
        let frames_to_send = 50;

        let mut handles = Vec::new();
        for _ in 0..num_subscribers {
            let (mut session, _) = gateway.subscribe(BIT_WEBSOCKET).expect("Subscribe failed");
            let handle = tokio::spawn(async move {
                let mut count = 0;
                while count < 30 {
                    if let Ok(frame) = tokio::time::timeout(Duration::from_millis(150), session.recv_frame()).await {
                        if frame.is_ok() {
                            count += 1;
                        }
                    } else {
                        break;
                    }
                }
                count
            });
            handles.push(handle);
        }

        tokio::time::sleep(Duration::from_millis(20)).await;

        // Producer task
        let gateway_prod = Arc::clone(&gateway);
        let prod_handle = tokio::spawn(async move {
            for i in 1..=frames_to_send {
                let frame = FramePayload::new_host(
                    i,
                    (i * 33_333) as u64,
                    640,
                    480,
                    1,
                    BIT_WEBSOCKET,
                    vec![i as u8; 100],
                );
                let flags = if i == 1 { FLAG_KEYFRAME } else { FLAG_DELTA_FRAME };
                let _ = gateway_prod.dispatch_frame(frame, flags);
                tokio::time::sleep(Duration::from_millis(2)).await;
            }
        });

        prod_handle.await.expect("Producer failed");

        for h in handles {
            let count = h.await.expect("Subscriber task failed");
            assert!(count > 0, "Expected subscriber to receive frames");
        }

        gateway.shutdown();
    }

    #[tokio::test]
    async fn test_async_graceful_shutdown() {
        let hub = Arc::new(BroadcastHub::new(16));
        let gateway = Arc::new(AsyncStreamGateway::new(hub, 16));

        let (mut session, _) = gateway.subscribe(BIT_WEBSOCKET).expect("Subscribe failed");

        let handle = tokio::spawn(async move {
            session.recv_frame().await
        });

        tokio::time::sleep(Duration::from_millis(20)).await;

        // Trigger graceful shutdown
        gateway.shutdown();

        // The receiver finishes with StreamError::GatewayClosed without hanging
        let result = tokio::time::timeout(Duration::from_millis(100), handle)
            .await
            .expect("Shutdown timed out")
            .expect("Task join error");

        assert_eq!(result, Err(StreamError::GatewayClosed));
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

    #[test]
    fn test_local_session_cache_unsafe_cell_borrow_free() {
        let cache = LocalSessionCache::new();
        assert!(cache.with_latest(|f| f.is_none()));

        let frame = FramePayload::new_host(
            100,
            1_000_000,
            640,
            480,
            1,
            BIT_WEBSOCKET,
            vec![255u8; 100],
        );

        cache.store(frame);

        let id = cache.with_latest(|f| {
            f.map(|frame_ref| frame_ref.frame_id).unwrap_or(0)
        });
        assert_eq!(id, 100);

        cache.clear();
        assert!(cache.with_latest(|f| f.is_none()));
    }

    #[test]
    fn test_gateway_runtime_builder_ticks_and_threads() {
        let rt = GatewayRuntimeBuilder::new()
            .worker_threads(2)
            .global_queue_interval(16)
            .thread_name("test-gw-worker")
            .build()
            .expect("Failed to build customized Tokio runtime");

        let res = rt.block_on(async {
            tokio::time::sleep(Duration::from_millis(5)).await;
            42
        });
        assert_eq!(res, 42);
    }

    #[tokio::test]
    async fn test_spawn_blocking_shm_bridge_to_async_gateway() {
        let stream_id = "test_shm_bridge_stream";
        let width = 32;
        let height = 32;
        let format = 1;
        let slots = 4;

        let mut writer = ShmWriter::create(stream_id, width, height, format, slots)
            .expect("Failed to create ShmWriter");

        let reader = ShmReader::open(stream_id)
            .expect("Failed to open ShmReader");

        let hub = Arc::new(BroadcastHub::new(16));
        let gateway = Arc::new(AsyncStreamGateway::new(hub, 16));

        let (mut session, _) = gateway.subscribe(BIT_WEBSOCKET)
            .expect("Failed to subscribe to gateway");

        // Write frame to SHM
        let payload = vec![77u8; 32 * 32 * 3];
        writer.write_frame(5_000_000, &payload).expect("Write frame failed");

        // Execute blocking SHM bridge
        let seq = spawn_blocking_shm_bridge(reader, Arc::clone(&gateway), BIT_WEBSOCKET, Some(Duration::from_secs(1)))
            .await
            .expect("SHM bridge failed");

        assert_eq!(seq, 1);

        // Async client receives frame seamlessly
        let frame = session.recv_frame().await.expect("Failed to recv frame from gateway");
        assert_eq!(frame.frame_id, 1);
        let buf = frame.host_buffer().expect("Expected host buffer");
        assert_eq!(buf.len(), 32 * 32 * 3);
        assert_eq!(buf[0], 77);

        gateway.shutdown();
    }

    #[tokio::test]
    async fn test_deadlock_prevention_with_tokio_timeout() {
        let sem = Arc::new(AtomicSemaphore::new(0)); // 0 permits initially

        // Attempting to acquire with a strict Tokio timeout should timeout cleanly without deadlock
        let sem_clone = Arc::clone(&sem);
        let acquire_future = async move {
            tokio::task::spawn_blocking(move || {
                sem_clone.acquire(Some(Duration::from_millis(50)))
            }).await.unwrap_or(false)
        };

        let result = tokio::time::timeout(Duration::from_millis(150), acquire_future)
            .await
            .expect("Timeout enclosing the test should not fire");

        assert!(!result, "Semaphore acquire should have timed out internally");
    }

    #[test]
    fn test_local_set_task_pinning_zero_task_stealing() {
        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .expect("Failed to build current_thread runtime");

        let local_set = tokio::task::LocalSet::new();

        let count = local_set.block_on(&rt, async {
            let cache = Arc::new(LocalSessionCache::new());
            let mut tasks = Vec::new();

            for i in 1..=5 {
                let cache_clone = Arc::clone(&cache);
                tasks.push(tokio::task::spawn_local(async move {
                    let frame = FramePayload::new_host(
                        i,
                        (i * 10_000) as u64,
                        320,
                        240,
                        1,
                        BIT_WEBSOCKET,
                        vec![i as u8; 50],
                    );
                    cache_clone.store(frame);
                    cache_clone.with_latest(|f| f.map(|f_ref| f_ref.frame_id).unwrap_or(0))
                }));
            }

            let mut sum = 0;
            for t in tasks {
                sum += t.await.unwrap_or(0);
            }
            sum
        });

        // The tasks ran on the pinned local thread set without inter-thread stealing
        assert!(count > 0);
    }

    #[tokio::test]
    async fn test_actor_model_gop_tracker_no_locks() {
        let actor = GopActorHandle::spawn(32);

        // Record a sequence of frames concurrently via Actor messages
        let mut handles = Vec::new();
        for i in 1..=20 {
            let actor_clone = actor.clone();
            handles.push(tokio::spawn(async move {
                let flags = if i == 1 || i == 15 { FLAG_KEYFRAME } else { FLAG_DELTA_FRAME };
                let payload = FramePayload::new_host(
                    i,
                    (i * 33_333) as u64,
                    640,
                    480,
                    1,
                    BIT_WEBSOCKET,
                    vec![i as u8; 50],
                );
                actor_clone.record_frame(i, flags, Some(payload)).await
            }));
        }

        for h in handles {
            h.await.expect("Actor task failed").expect("Send message failed");
        }

        // Query Actor state via oneshot reply channel
        let latest_kf = actor.get_latest_keyframe().await.expect("Get latest keyframe failed");
        assert!(latest_kf.is_some());
        assert_eq!(latest_kf.unwrap().frame_id, 15);

        let stats = actor.get_gop_stats().await.expect("Get GOP stats failed");
        assert_eq!(stats.latest_keyframe_seq, 15);
        assert_eq!(stats.total_frames_tracked, 20);

        actor.shutdown().await;
    }

    #[test]
    fn test_symmetric_coroutines_pipeline_chain() {
        let nal_coro = NalClassifierCoroutine::new(false);
        let fps_coro = FpsFilterCoroutine::new(10.0); // 10 FPS (100,000 us interval)

        // Symmetrically chain coroutines in the same stack frame
        let mut pipeline_coro = nal_coro.then(fps_coro);

        let idr_bytes = vec![0x00, 0x00, 0x00, 0x01, 0x65, 0x88];
        let p_bytes = vec![0x00, 0x00, 0x00, 0x01, 0x41, 0x9A];

        let f1 = FramePayload::new_host(1, 0, 640, 480, 1, BIT_ALL, idr_bytes);
        let f2 = FramePayload::new_host(2, 30_000, 640, 480, 1, BIT_ALL, p_bytes.clone());
        let f3 = FramePayload::new_host(3, 110_000, 640, 480, 1, BIT_ALL, p_bytes);

        // Frame 1: Keyframe -> should pass
        match pipeline_coro.resume(f1) {
            CoroutineYield::Continue((_, flags)) => {
                assert_eq!(flags & FLAG_KEYFRAME, FLAG_KEYFRAME);
            }
            CoroutineYield::Skip => panic!("Keyframe should not be skipped"),
        }

        // Frame 2: 30ms -> too soon for 10 FPS -> should skip
        match pipeline_coro.resume(f2) {
            CoroutineYield::Continue(_) => panic!("Frame 2 should have been skipped by FPS filter"),
            CoroutineYield::Skip => {}
        }

        // Frame 3: 110ms -> elapsed > 100ms -> should pass
        match pipeline_coro.resume(f3) {
            CoroutineYield::Continue((payload, _)) => {
                assert_eq!(payload.frame_id, 3);
            }
            CoroutineYield::Skip => panic!("Frame 3 should have passed"),
        }
    }

    #[test]
    fn test_circuit_breaker_fast_fail_and_recovery() {
        let cb = StreamCircuitBreaker::new(3, Duration::from_millis(50));
        assert_eq!(cb.state(), CircuitState::Closed);
        assert!(cb.allow_request());

        // 3 consecutive failures trigger OPEN state
        cb.record_failure();
        cb.record_failure();
        cb.record_failure();

        assert_eq!(cb.state(), CircuitState::Open);
        // Fast-fail: Immediate rejection without creating tasks
        assert!(!cb.allow_request());

        // Wait for cooldown
        std::thread::sleep(Duration::from_millis(60));

        // After cooldown, transitions to HalfOpen on request
        assert!(cb.allow_request());
        assert_eq!(cb.state(), CircuitState::HalfOpen);

        // Successful probe requests recover the circuit back to Closed
        cb.record_success();
        cb.record_success();
        cb.record_success();

        assert_eq!(cb.state(), CircuitState::Closed);
        assert!(cb.allow_request());
    }

    #[tokio::test]
    async fn test_future_decorator_telemetry_profiling() {
        let sink = Arc::new(FutureMetricsSink::new());

        let async_task = async {
            tokio::time::sleep(Duration::from_millis(10)).await;
            123
        };

        // Wrap future with TelemetryFuture decorator
        let decorated = async_task.with_telemetry(Arc::clone(&sink));
        let result = decorated.await;
        assert_eq!(result, 123);

        let (polls, completions, latency_nanos) = sink.stats();
        assert!(polls >= 1, "Expected at least 1 poll");
        assert_eq!(completions, 1);
        assert!(latency_nanos > 0, "Expected measured latency");
    }

    #[test]
    fn test_isolated_module_pattern_sync_bridge() {
        let rt = tokio::runtime::Builder::new_multi_thread()
            .worker_threads(2)
            .enable_all()
            .build()
            .expect("Failed to build Tokio runtime");

        let hub = Arc::new(BroadcastHub::new(16));
        let gateway = Arc::new(AsyncStreamGateway::new(hub, 16));
        let sync_bridge = SyncGatewayBridge::new(gateway, rt.handle().clone());

        // Subscribe synchronously - returns crossbeam SyncReceiver without async/await
        let (sync_rx, _) = sync_bridge.subscribe_sync(BIT_WEBSOCKET, 8)
            .expect("Failed to subscribe synchronously");

        assert_eq!(sync_bridge.active_clients_sync(), 1);

        // Dispatch synchronously from caller thread
        let payload = FramePayload::new_host(
            999,
            1_000_000,
            640,
            480,
            1,
            BIT_WEBSOCKET,
            vec![88u8; 100],
        );
        let dispatched = sync_bridge.dispatch_frame_sync(payload, 0).expect("Dispatch sync failed");
        assert!(dispatched > 0);

        // Synchronous receive without async keywords
        let received = sync_rx.recv_timeout(Duration::from_secs(1))
            .expect("Sync receive timed out");
        assert_eq!(received.frame_id, 999);

        sync_bridge.shutdown_sync();
    }
}


