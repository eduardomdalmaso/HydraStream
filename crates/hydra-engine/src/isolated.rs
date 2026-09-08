//! Isolated Module Pattern for Synchronous Data Plane Encapsulation
//! ("Async Rust" - Chapter 1 & 2: Preserving Function Purity & Eliminating Async Contagion)
//!
//! Encapsulates the async Tokio runtime and `AsyncStreamGateway` behind a clean,
//! 100% synchronous interface (`SyncGatewayBridge`). Prevents "function color" pollution.

use std::sync::Arc;
use std::time::Duration;
use crossbeam_channel::{Receiver, Sender};
use crate::gateway::{AsyncStreamGateway, StreamError};
use crate::transport::FramePayload;

/// Synchronous Receiver channel yielding frames without async/await syntax.
pub struct SyncReceiver {
    rx: Receiver<FramePayload>,
    client_id: usize,
    _task_handle: tokio::task::JoinHandle<()>,
}

impl SyncReceiver {
    /// Synchronously receives the next frame with a timeout (zero async pollution).
    pub fn recv_timeout(&self, timeout: Duration) -> Result<FramePayload, StreamError> {
        self.rx.recv_timeout(timeout).map_err(|_| StreamError::Timeout)
    }

    /// Synchronously tries to receive a frame without blocking.
    pub fn try_recv(&self) -> Result<FramePayload, StreamError> {
        self.rx.try_recv().map_err(|_| StreamError::BufferOverflow(0))
    }

    pub fn client_id(&self) -> usize {
        self.client_id
    }
}

/// Synchronous Gateway Bridge encapsulating the async runtime.
pub struct SyncGatewayBridge {
    gateway: Arc<AsyncStreamGateway>,
    rt_handle: tokio::runtime::Handle,
}

impl SyncGatewayBridge {
    pub fn new(gateway: Arc<AsyncStreamGateway>, rt_handle: tokio::runtime::Handle) -> Self {
        Self {
            gateway,
            rt_handle,
        }
    }

    /// Synchronously dispatches a frame from pure synchronous data plane code.
    #[inline(always)]
    pub fn dispatch_frame_sync(&self, frame: FramePayload, flags: u32) -> Result<usize, StreamError> {
        self.gateway.dispatch_frame(frame, flags)
    }

    /// Subscribes a client and returns a synchronous channel (`SyncReceiver`).
    pub fn subscribe_sync(&self, interest_mask: u32, channel_bound: usize) -> Result<(SyncReceiver, Option<FramePayload>), StreamError> {
        let (mut async_session, initial_kf) = self.gateway.subscribe(interest_mask)?;
        let client_id = async_session.client_id;
        let (tx, rx): (Sender<FramePayload>, Receiver<FramePayload>) = crossbeam_channel::bounded(channel_bound.max(4));

        // Background async bridge task running inside Tokio
        let task_handle = self.rt_handle.spawn(async move {
            while let Ok(frame) = async_session.recv_frame().await {
                if tx.send(frame).is_err() {
                    break;
                }
            }
        });

        let sync_receiver = SyncReceiver {
            rx,
            client_id,
            _task_handle: task_handle,
        };

        Ok((sync_receiver, initial_kf))
    }

    pub fn active_clients_sync(&self) -> usize {
        self.gateway.active_clients()
    }

    pub fn shutdown_sync(&self) {
        self.gateway.shutdown();
    }
}
