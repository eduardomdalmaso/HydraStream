//! RTMP connection handler and session lifecycle management

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Duration;

use tokio::net::{TcpListener, TcpStream};

use crate::gateway::AsyncStreamGateway;
use crate::server::StreamRegistry;
use crate::shm::ShmWriter;

use super::amf::handle_amf0_command;
use super::chunk::*;
use super::handshake::rtmp_handshake;
use super::video::handle_video_message;

pub struct RtmpServer {
    addr: SocketAddr,
    registry: Arc<StreamRegistry>,
    is_running: Arc<AtomicBool>,
}

impl RtmpServer {
    pub fn new(addr: SocketAddr, registry: Arc<StreamRegistry>) -> Self {
        Self {
            addr,
            registry,
            is_running: Arc::new(AtomicBool::new(true)),
        }
    }

    pub fn shutdown(&self) {
        self.is_running.store(false, Ordering::SeqCst);
    }

    pub async fn run(&self) -> Result<(), std::io::Error> {
        let listener = TcpListener::bind(self.addr).await?;
        println!("🚀 [Hydra RTMP Ingest] Listening for RTMP push on rtmp://{}", self.addr);

        while self.is_running.load(Ordering::Relaxed) {
            match listener.accept().await {
                Ok((stream, peer_addr)) => {
                    let reg = Arc::clone(&self.registry);
                    tokio::spawn(async move {
                        if let Err(e) = handle_rtmp_connection(stream, peer_addr, reg).await {
                            eprintln!("⚠️ [Hydra RTMP] Connection error from {}: {}", peer_addr, e);
                        }
                    });
                }
                Err(e) => {
                    if !self.is_running.load(Ordering::Relaxed) {
                        break;
                    }
                    eprintln!("⚠️ [Hydra RTMP] Accept error: {}", e);
                    tokio::time::sleep(Duration::from_millis(50)).await;
                }
            }
        }
        Ok(())
    }
}

pub struct RtmpSessionState {
    pub in_chunk_size: usize,
    pub out_chunk_size: usize,
    pub chunk_states: HashMap<u32, ChunkStreamState>,
    pub stream_id: Option<String>,
    pub cached_seq_header: Option<Vec<u8>>,
    pub shm_writer: Option<ShmWriter>,
    pub gateway: Option<Arc<AsyncStreamGateway>>,
    pub frame_seq: u64,
}

impl RtmpSessionState {
    pub fn new() -> Self {
        Self {
            in_chunk_size: RTMP_DEFAULT_CHUNK_SIZE,
            out_chunk_size: RTMP_SERVER_OUT_CHUNK_SIZE,
            chunk_states: HashMap::new(),
            stream_id: None,
            cached_seq_header: None,
            shm_writer: None,
            gateway: None,
            frame_seq: 0,
        }
    }
}

pub async fn handle_rtmp_connection(
    mut stream: TcpStream,
    peer_addr: SocketAddr,
    registry: Arc<StreamRegistry>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    rtmp_handshake(&mut stream).await?;

    let mut state = RtmpSessionState::new();
    let mut stream_id_resolved = String::new();

    loop {
        let msg = match read_rtmp_message(&mut stream, &mut state.chunk_states, state.in_chunk_size).await? {
            Some(m) => m,
            None => break,
        };

        match msg.msg_type_id {
            MSG_SET_CHUNK_SIZE => {
                if msg.payload.len() >= 4 {
                    let new_size = u32::from_be_bytes(msg.payload[0..4].try_into()?) as usize;
                    if new_size > 0 {
                        state.in_chunk_size = new_size;
                    }
                }
            }
            MSG_AMF0_COMMAND => {
                handle_amf0_command(&mut stream, &mut state, &msg, &registry).await?;
                if let Some(ref sid) = state.stream_id {
                    if stream_id_resolved.is_empty() {
                        stream_id_resolved = sid.clone();
                    }
                }
            }
            MSG_VIDEO => {
                handle_video_message(&mut state, &msg)?;
            }
            _ => {}
        }
    }

    if !stream_id_resolved.is_empty() {
        println!("🛑 [Hydra RTMP] Publisher disconnected from '{}' ({})", stream_id_resolved, peer_addr);
        registry.unregister(&stream_id_resolved);
    }
    Ok(())
}
