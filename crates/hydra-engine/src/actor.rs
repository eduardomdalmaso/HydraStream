//! Actor Model for Video GOP Tracking & Client State Management
//! ("Async Rust" - Chapter 7: The Actor Pattern vs Mutex Contention)
//!
//! Replaces async `Mutex<GopState>` with an isolated Actor processing sequential messages
//! via bounded `mpsc` and replying over `oneshot` channels. Eliminates lock contention.

use std::sync::atomic::{AtomicU64, Ordering};
use tokio::sync::{mpsc, oneshot};
use crate::codec::FLAG_KEYFRAME;
use crate::gateway::StreamError;
use crate::transport::FramePayload;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct GopStats {
    pub latest_keyframe_seq: u64,
    pub gop_size: u64,
    pub total_frames_tracked: u64,
}

pub enum GopMessage {
    RecordFrame {
        frame_id: u64,
        flags: u32,
        payload: Option<FramePayload>,
    },
    GetLatestKeyframe {
        reply: oneshot::Sender<Option<FramePayload>>,
    },
    GetGopStats {
        reply: oneshot::Sender<GopStats>,
    },
    Shutdown,
}

/// The state managed privately by the GopActor (zero shared locks).
struct GopActorState {
    latest_keyframe_seq: u64,
    latest_keyframe: Option<FramePayload>,
    gop_size: u64,
    frames_since_keyframe: u64,
    total_frames: u64,
}

impl GopActorState {
    fn new() -> Self {
        Self {
            latest_keyframe_seq: 0,
            latest_keyframe: None,
            gop_size: 0,
            frames_since_keyframe: 0,
            total_frames: 0,
        }
    }

    fn record_frame(&mut self, frame_id: u64, flags: u32, payload: Option<FramePayload>) {
        self.total_frames += 1;
        let is_keyframe = (flags & FLAG_KEYFRAME) != 0;

        if is_keyframe {
            if self.latest_keyframe_seq > 0 {
                self.gop_size = self.frames_since_keyframe;
            }
            self.latest_keyframe_seq = frame_id;
            self.frames_since_keyframe = 1;
            if let Some(p) = payload {
                self.latest_keyframe = Some(p);
            }
        } else {
            self.frames_since_keyframe += 1;
        }
    }
}

/// Handle used by concurrent tasks to interact with the GopActor.
#[derive(Clone)]
pub struct GopActorHandle {
    sender: mpsc::Sender<GopMessage>,
    total_dispatched: std::sync::Arc<AtomicU64>,
}

impl GopActorHandle {
    /// Spawns a new GopActor on the active Tokio runtime.
    pub fn spawn(capacity: usize) -> Self {
        let (sender, mut receiver) = mpsc::channel(capacity.max(32));
        let total_dispatched = std::sync::Arc::new(AtomicU64::new(0));

        tokio::spawn(async move {
            let mut state = GopActorState::new();
            while let Some(msg) = receiver.recv().await {
                match msg {
                    GopMessage::RecordFrame { frame_id, flags, payload } => {
                        state.record_frame(frame_id, flags, payload);
                    }
                    GopMessage::GetLatestKeyframe { reply } => {
                        let _ = reply.send(state.latest_keyframe.clone());
                    }
                    GopMessage::GetGopStats { reply } => {
                        let stats = GopStats {
                            latest_keyframe_seq: state.latest_keyframe_seq,
                            gop_size: state.gop_size,
                            total_frames_tracked: state.total_frames,
                        };
                        let _ = reply.send(stats);
                    }
                    GopMessage::Shutdown => {
                        break;
                    }
                }
            }
        });

        Self {
            sender,
            total_dispatched,
        }
    }

    /// Asynchronously records a frame into the Actor loop without lock contention.
    pub async fn record_frame(&self, frame_id: u64, flags: u32, payload: Option<FramePayload>) -> Result<(), StreamError> {
        self.total_dispatched.fetch_add(1, Ordering::Relaxed);
        self.sender
            .send(GopMessage::RecordFrame { frame_id, flags, payload })
            .await
            .map_err(|_| StreamError::GatewayClosed)
    }

    /// Requests the latest keyframe using a dedicated `oneshot` reply channel.
    pub async fn get_latest_keyframe(&self) -> Result<Option<FramePayload>, StreamError> {
        let (reply_tx, reply_rx) = oneshot::channel();
        self.sender
            .send(GopMessage::GetLatestKeyframe { reply: reply_tx })
            .await
            .map_err(|_| StreamError::GatewayClosed)?;

        reply_rx.await.map_err(|_| StreamError::GatewayClosed)
    }

    /// Requests GOP statistics from the Actor.
    pub async fn get_gop_stats(&self) -> Result<GopStats, StreamError> {
        let (reply_tx, reply_rx) = oneshot::channel();
        self.sender
            .send(GopMessage::GetGopStats { reply: reply_tx })
            .await
            .map_err(|_| StreamError::GatewayClosed)?;

        reply_rx.await.map_err(|_| StreamError::GatewayClosed)
    }

    /// Closes the Actor gracefully.
    pub async fn shutdown(&self) {
        let _ = self.sender.send(GopMessage::Shutdown).await;
    }
}
