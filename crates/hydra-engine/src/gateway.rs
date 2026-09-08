use std::cell::UnsafeCell;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU64, AtomicUsize, Ordering};
use std::time::Duration;
use tokio::sync::broadcast;
use crate::codec::{GopTracker, FLAG_KEYFRAME};
use crate::transport::{BroadcastHub, FramePayload};
use crate::shm::ShmReader;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StreamError {
    GatewayClosed,
    BufferOverflow(u64), // Number of skipped/lagged frames
    ClientNotFound,
    Timeout,
    IoError,
}

impl std::fmt::Display for StreamError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::GatewayClosed => write!(f, "Stream gateway is shut down"),
            Self::BufferOverflow(n) => write!(f, "Client lagged behind; skipped {} stale frame(s)", n),
            Self::ClientNotFound => write!(f, "Client ID not found or already inactive"),
            Self::Timeout => write!(f, "Async operation timed out"),
            Self::IoError => write!(f, "I/O or SHM Bridge error"),
        }
    }
}

impl std::error::Error for StreamError {}

/// High-speed single-threaded session cache using `UnsafeCell`
/// Eliminates runtime `RefCell` borrow-check overhead.
/// INVARIANT: References yielded by this cache MUST NEVER be held across `.await` points!
pub struct LocalSessionCache {
    cache: UnsafeCell<Option<FramePayload>>,
}

// Safety: LocalSessionCache is designed for thread-local usage or within pinned local tasks.
unsafe impl Sync for LocalSessionCache {}

impl LocalSessionCache {
    pub fn new() -> Self {
        Self {
            cache: UnsafeCell::new(None),
        }
    }

    /// Stores the latest frame into the local cache with zero borrow-checker runtime penalty.
    #[inline(always)]
    pub fn store(&self, frame: FramePayload) {
        unsafe {
            *self.cache.get() = Some(frame);
        }
    }

    /// Accesses the cached frame synchronously without holding references across `.await`.
    #[inline(always)]
    pub fn with_latest<R, F: FnOnce(Option<&FramePayload>) -> R>(&self, f: F) -> R {
        unsafe {
            let ptr = self.cache.get();
            f((*ptr).as_ref())
        }
    }

    /// Clears the cached frame.
    #[inline(always)]
    pub fn clear(&self) {
        unsafe {
            *self.cache.get() = None;
        }
    }
}

impl Default for LocalSessionCache {
    fn default() -> Self {
        Self::new()
    }
}

/// Builder for customized Tokio Runtime tuned for low-latency video streaming
/// ("Async Rust" - Chapter 9: Scheduler Ticks & Thread Pool Tuning)
pub struct GatewayRuntimeBuilder {
    worker_threads: usize,
    global_queue_interval: u32,
    thread_name: String,
}

impl GatewayRuntimeBuilder {
    pub fn new() -> Self {
        Self {
            worker_threads: num_cpus_fallback(),
            global_queue_interval: 16, // Low-latency tick default (standard is 61)
            thread_name: "hydra-gateway-worker".to_string(),
        }
    }

    pub fn worker_threads(mut self, threads: usize) -> Self {
        self.worker_threads = threads.max(1);
        self
    }

    /// Sets the interval at which the scheduler checks the global queue.
    /// Lower values (e.g. 16 or 31 vs default 61) decrease task starvation under high I/O ingest.
    pub fn global_queue_interval(mut self, interval: u32) -> Self {
        self.global_queue_interval = interval.max(1);
        self
    }

    pub fn thread_name(mut self, name: impl Into<String>) -> Self {
        self.thread_name = name.into();
        self
    }

    pub fn build(self) -> std::io::Result<tokio::runtime::Runtime> {
        tokio::runtime::Builder::new_multi_thread()
            .worker_threads(self.worker_threads)
            .global_queue_interval(self.global_queue_interval)
            .thread_name(self.thread_name)
            .enable_all()
            .build()
    }
}

impl Default for GatewayRuntimeBuilder {
    fn default() -> Self {
        Self::new()
    }
}

fn num_cpus_fallback() -> usize {
    std::thread::available_parallelism()
        .map(|n| n.get())
        .unwrap_or(4)
}

/// Async client subscription session with Cancellation Safety & Lagged Recovery
pub struct ClientSession {
    pub client_id: usize,
    pub interest_mask: u32,
    pub receiver: broadcast::Receiver<FramePayload>,
    hub: Arc<BroadcastHub>,
}

impl ClientSession {
    /// Asynchronously receives the next frame.
    /// If the client lagged behind, recovers automatically by jumping forward to the latest frame.
    pub async fn recv_frame(&mut self) -> Result<FramePayload, StreamError> {
        loop {
            match self.receiver.recv().await {
                Ok(frame) => {
                    if (frame.mask & self.interest_mask) != 0 {
                        return Ok(frame);
                    }
                }
                Err(broadcast::error::RecvError::Lagged(_skipped)) => {
                    // Backpressure recovery: Skip stale frames and continue listening
                    continue;
                }
                Err(broadcast::error::RecvError::Closed) => {
                    return Err(StreamError::GatewayClosed);
                }
            }
        }
    }
}

// Cancellation Safety: If an async task is aborted or dropped, cleanly unregister peer
impl Drop for ClientSession {
    fn drop(&mut self) {
        self.hub.peers.mark_inactive(self.client_id);
    }
}

/// Async Stream Gateway multiplexing frames from the zero-copy engine to async network tasks
pub struct AsyncStreamGateway {
    hub: Arc<BroadcastHub>,
    gop_tracker: Arc<GopTracker>,
    tx: parking_lot::RwLock<Option<broadcast::Sender<FramePayload>>>,
    latest_keyframe: parking_lot::RwLock<Option<FramePayload>>,
    next_client_id: AtomicUsize,
    total_clients_served: AtomicU64,
    is_running: AtomicBool,
}

impl AsyncStreamGateway {
    pub fn new(hub: Arc<BroadcastHub>, channel_capacity: usize) -> Self {
        let (tx, _) = broadcast::channel(channel_capacity.max(2));
        Self {
            hub,
            gop_tracker: Arc::new(GopTracker::new()),
            tx: parking_lot::RwLock::new(Some(tx)),
            latest_keyframe: parking_lot::RwLock::new(None),
            next_client_id: AtomicUsize::new(1),
            total_clients_served: AtomicU64::new(0),
            is_running: AtomicBool::new(true),
        }
    }

    /// Publishes a frame to the async broadcast channel and updates GOP tracking.
    /// Returns the number of active async receivers that received the frame.
    pub fn dispatch_frame(&self, frame: FramePayload, flags: u32) -> Result<usize, StreamError> {
        if !self.is_running.load(Ordering::Acquire) {
            return Err(StreamError::GatewayClosed);
        }

        let is_keyframe = (flags & FLAG_KEYFRAME) != 0;
        
        self.gop_tracker.record_frame(frame.frame_id, flags);

        if is_keyframe {
            let mut lock = self.latest_keyframe.write();
            *lock = Some(frame.clone());
        }

        // Also publish to low-level sync BroadcastHub
        self.hub.publish(frame.clone());

        // Broadcast to all active Tokio WebSocket/WebRTC receiver tasks
        let lock = self.tx.read();
        match &*lock {
            Some(sender) => match sender.send(frame) {
                Ok(receiver_count) => Ok(receiver_count),
                Err(_) => Ok(0),
            },
            None => Err(StreamError::GatewayClosed),
        }
    }

    /// Subscribes a new async WebSocket / WebRTC client.
    /// Returns the session along with the latest Keyframe (if available) for instant startup.
    pub fn subscribe(&self, interest_mask: u32) -> Result<(ClientSession, Option<FramePayload>), StreamError> {
        if !self.is_running.load(Ordering::Acquire) {
            return Err(StreamError::GatewayClosed);
        }

        let receiver = {
            let lock = self.tx.read();
            match &*lock {
                Some(sender) => sender.subscribe(),
                None => return Err(StreamError::GatewayClosed),
            }
        };

        let client_id = self.next_client_id.fetch_add(1, Ordering::Relaxed);
        self.total_clients_served.fetch_add(1, Ordering::Relaxed);

        // Register in low-level lock-free peer list
        self.hub.peers.register(client_id, interest_mask);

        let initial_keyframe = {
            let lock = self.latest_keyframe.read();
            (*lock).clone()
        };

        let session = ClientSession {
            client_id,
            interest_mask,
            receiver,
            hub: Arc::clone(&self.hub),
        };

        Ok((session, initial_keyframe))
    }

    pub fn active_clients(&self) -> usize {
        self.hub.peers.active_count()
    }

    pub fn gop_tracker(&self) -> &GopTracker {
        &self.gop_tracker
    }

    pub fn is_running(&self) -> bool {
        self.is_running.load(Ordering::Acquire)
    }

    /// Graceful Shutdown: Drops the broadcast sender to close channel immediately
    pub fn shutdown(&self) {
        self.is_running.store(false, Ordering::Release);
        {
            let mut lock = self.tx.write();
            *lock = None;
        }
        self.hub.close();
    }
}

/// Bridges synchronous/blocking SHM reader to Tokio Async Gateway using `spawn_blocking`.
/// Guarantees that Linux Futex / SHM wait calls never block Tokio async worker threads.
pub async fn spawn_blocking_shm_bridge(
    mut reader: ShmReader,
    gateway: Arc<AsyncStreamGateway>,
    interest_mask: u32,
    timeout: Option<Duration>,
) -> Result<u64, StreamError> {
    tokio::task::spawn_blocking(move || {
        let mut buf = Vec::new();
        let meta = reader.wait_and_read_frame(&mut buf, timeout)
            .map_err(|_| StreamError::IoError)?
            .ok_or(StreamError::Timeout)?;

        let payload = FramePayload::new_host(
            meta.sequence,
            meta.timestamp_us,
            reader.header().width,
            reader.header().height,
            reader.header().format,
            interest_mask,
            buf,
        );

        gateway.dispatch_frame(payload, 0)?;
        Ok(meta.sequence)
    })
    .await
    .map_err(|_| StreamError::GatewayClosed)?
}

