//! Async Video Stream Gateway powered by Tokio
//! Bridges the low-level synchronous zero-copy Data Plane with asynchronous WebSocket/WebRTC tasks.
//! Adheres to Effective Rust guidelines for error propagation (Result<T, StreamError>) and strict lock scoping.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU64, AtomicUsize, Ordering};
use tokio::sync::broadcast;
use crate::codec::{GopTracker, FLAG_KEYFRAME};
use crate::transport::{BroadcastHub, FramePayload};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StreamError {
    GatewayClosed,
    BufferOverflow,
    ClientNotFound,
}

impl std::fmt::Display for StreamError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::GatewayClosed => write!(f, "Stream gateway is shut down"),
            Self::BufferOverflow => write!(f, "Receiver buffer capacity exceeded"),
            Self::ClientNotFound => write!(f, "Client ID not found or already inactive"),
        }
    }
}

impl std::error::Error for StreamError {}

/// Async client subscription session
pub struct ClientSession {
    pub client_id: usize,
    pub interest_mask: u32,
    pub receiver: broadcast::Receiver<FramePayload>,
}

/// Async Stream Gateway multiplexing frames from the zero-copy engine to async network tasks
pub struct AsyncStreamGateway {
    hub: Arc<BroadcastHub>,
    gop_tracker: Arc<GopTracker>,
    tx: broadcast::Sender<FramePayload>,
    latest_keyframe: parking_lot::RwLock<Option<FramePayload>>,
    next_client_id: AtomicUsize,
    total_clients_served: AtomicU64,
    is_running: AtomicBool,
}

impl AsyncStreamGateway {
    pub fn new(hub: Arc<BroadcastHub>, channel_capacity: usize) -> Self {
        let (tx, _) = broadcast::channel(channel_capacity.max(32));
        Self {
            hub,
            gop_tracker: Arc::new(GopTracker::new()),
            tx,
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
            // Lock scope strictly minimized (Item 17 of Effective Rust)
            let mut lock = self.latest_keyframe.write();
            *lock = Some(frame.clone());
        }

        // Also publish to low-level sync BroadcastHub
        self.hub.publish(frame.clone());

        // Broadcast to all active Tokio WebSocket/WebRTC receiver tasks
        match self.tx.send(frame) {
            Ok(receiver_count) => Ok(receiver_count),
            Err(_) => Ok(0), // No active async receivers at this instant
        }
    }

    /// Subscribes a new async WebSocket / WebRTC client.
    /// Returns the session along with the latest Keyframe (if available) for instant startup.
    pub fn subscribe(&self, interest_mask: u32) -> Result<(ClientSession, Option<FramePayload>), StreamError> {
        if !self.is_running.load(Ordering::Acquire) {
            return Err(StreamError::GatewayClosed);
        }

        let client_id = self.next_client_id.fetch_add(1, Ordering::Relaxed);
        self.total_clients_served.fetch_add(1, Ordering::Relaxed);

        // Register in low-level lock-free peer list
        self.hub.peers.register(client_id, interest_mask);

        let receiver = self.tx.subscribe();
        let initial_keyframe = {
            let lock = self.latest_keyframe.read();
            (*lock).clone()
        };

        let session = ClientSession {
            client_id,
            interest_mask,
            receiver,
        };

        Ok((session, initial_keyframe))
    }

    /// Unsubscribes a client on disconnect
    pub fn unsubscribe(&self, client_id: usize) {
        self.hub.peers.mark_inactive(client_id);
    }

    pub fn active_clients(&self) -> usize {
        self.hub.peers.active_count()
    }

    pub fn gop_tracker(&self) -> &GopTracker {
        &self.gop_tracker
    }

    pub fn shutdown(&self) {
        self.is_running.store(false, Ordering::Release);
        self.hub.close();
    }
}
