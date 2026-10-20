//! Dedicated Tokio WebSocket Server for Binary WSP Stream Delivery
//! High-throughput zero-GC video frame egress for W3C WebCodecs consumers.

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use futures_util::{SinkExt, StreamExt};
use parking_lot::RwLock;
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::broadcast;
use tokio_tungstenite::tungstenite::protocol::Message;
use tokio_tungstenite::tungstenite::handshake::server::{Request, Response};
use crate::codec::FLAG_KEYFRAME;
use crate::gateway::AsyncStreamGateway;
use crate::transcode::TranscodePool;
use crate::transport::BIT_WEBSOCKET;
use crate::wsp::pack_wsp_frame;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StreamCodec {
    H264,
    H265,
}

pub struct StreamEntry {
    pub stream_id: String,
    pub codec: StreamCodec,
    pub gateway: Arc<AsyncStreamGateway>,
}

/// Registry storing active stream gateways and transcoder pool
pub struct StreamRegistry {
    streams: RwLock<HashMap<String, Arc<StreamEntry>>>,
    transcode_pool: Arc<TranscodePool>,
}

impl StreamRegistry {
    pub fn new() -> Self {
        Self {
            streams: RwLock::new(HashMap::new()),
            transcode_pool: Arc::new(TranscodePool::new()),
        }
    }

    pub fn register(&self, stream_id: &str, codec: StreamCodec, gateway: Arc<AsyncStreamGateway>) {
        let entry = Arc::new(StreamEntry {
            stream_id: stream_id.to_string(),
            codec,
            gateway,
        });
        self.streams.write().insert(stream_id.to_string(), entry);
    }

    pub fn unregister(&self, stream_id: &str) {
        self.streams.write().remove(stream_id);
    }

    pub fn unregister_if_gateway_matches(&self, stream_id: &str, gateway: &Arc<AsyncStreamGateway>) {
        let mut map = self.streams.write();
        if let Some(entry) = map.get(stream_id) {
            if Arc::ptr_eq(&entry.gateway, gateway) {
                map.remove(stream_id);
            }
        }
    }

    pub fn get(&self, stream_id: &str) -> Option<Arc<StreamEntry>> {
        self.streams.read().get(stream_id).cloned()
    }

    pub fn transcode_pool(&self) -> Arc<TranscodePool> {
        self.transcode_pool.clone()
    }
}

impl Default for StreamRegistry {
    fn default() -> Self {
        Self::new()
    }
}

/// Dedicated WSP WebSocket Server
pub struct WspServer {
    addr: SocketAddr,
    registry: Arc<StreamRegistry>,
    is_running: Arc<AtomicBool>,
}

impl WspServer {
    pub fn new(addr: SocketAddr, registry: Arc<StreamRegistry>) -> Self {
        Self {
            addr,
            registry,
            is_running: Arc::new(AtomicBool::new(true)),
        }
    }

    pub async fn run(&self) -> Result<(), std::io::Error> {
        let listener = TcpListener::bind(self.addr).await?;
        println!("🚀 [Hydra WSP Server] Listening for WebCodecs clients on ws://{}", self.addr);

        while self.is_running.load(Ordering::Relaxed) {
            match listener.accept().await {
                Ok((stream, peer_addr)) => {
                    let registry = self.registry.clone();
                    tokio::spawn(async move {
                        if let Err(e) = handle_connection(stream, peer_addr, registry).await {
                            eprintln!("⚠️ [Hydra WSP] Client {} error: {}", peer_addr, e);
                        }
                    });
                }
                Err(e) => {
                    if !self.is_running.load(Ordering::Relaxed) {
                        break;
                    }
                    eprintln!("⚠️ [Hydra WSP Server] Accept error: {}", e);
                }
            }
        }
        Ok(())
    }

    pub fn stop(&self) {
        self.is_running.store(false, Ordering::Release);
    }
}

fn parse_stream_path(uri: &str) -> Option<String> {
    let path = uri.split('?').next().unwrap_or(uri).trim_matches('/');
    let parts: Vec<&str> = path.split('/').collect();
    if parts.is_empty() || parts[0].is_empty() { return None; }
    if parts.len() >= 2 && parts[0] == "live" { return Some(parts[1].to_string()); }
    if parts.len() >= 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "streams" {
        return Some(parts[3].to_string());
    }
    if parts.len() == 1 { return Some(parts[0].to_string()); }
    None
}

fn parse_query_param(uri: &str, param: &str) -> Option<String> {
    uri.split('?').nth(1)?.split('&').find_map(|pair| {
        let mut kv = pair.split('=');
        if kv.next()? == param { Some(kv.next()?.to_string()) } else { None }
    })
}

pub async fn handle_connection(
    stream: TcpStream,
    peer_addr: SocketAddr,
    registry: Arc<StreamRegistry>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let mut requested_uri = String::new();

    let callback = |req: &Request, resp: Response| {
        requested_uri = req.uri().to_string();
        Ok(resp)
    };

    let ws_stream = tokio_tungstenite::accept_hdr_async(stream, callback).await?;
    let (mut ws_tx, mut ws_rx) = ws_stream.split();

    let stream_id = match parse_stream_path(&requested_uri) {
        Some(id) => id,
        None => {
            let _ = ws_tx.send(Message::Close(None)).await;
            return Ok(());
        }
    };

    let requested_codec = parse_query_param(&requested_uri, "codec")
        .unwrap_or_else(|| "raw".to_string());
    let requested_profile = parse_query_param(&requested_uri, "profile")
        .unwrap_or_else(|| "main".to_string());

    let target_stream_id = if requested_profile == "sub" {
        let sub_id = format!("{}_sub", stream_id);
        if registry.get(&sub_id).is_some() { sub_id } else { stream_id.clone() }
    } else {
        stream_id.clone()
    };

    let mut stream_entry = None;
    for _ in 0..40 {
        if let Some(e) = registry.get(&target_stream_id) {
            stream_entry = Some(e);
            break;
        }
        tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
    }

    let stream_entry = match stream_entry {
        Some(e) => e,
        None => {
            let _ = ws_tx.send(Message::Text(format!(r#"{{"error":"stream_not_found","stream_id":"{}"}}"#, target_stream_id))).await;
            let _ = ws_tx.send(Message::Close(None)).await;
            return Ok(());
        }
    };

    println!(
        "📡 [Hydra WSP] Client {} connected to '{}' (profile: {}, requested codec: {})",
        peer_addr, target_stream_id, requested_profile, requested_codec
    );

    // Branch 1: Transcoding requested (H.265 stream requesting H.264)
    if stream_entry.codec == StreamCodec::H265 && requested_codec == "h264" {
        let transcode_pool = registry.transcode_pool();
        let worker = match transcode_pool.get_or_spawn(&target_stream_id) {
            Ok(w) => w,
            Err(e) => {
                let _ = ws_tx.send(Message::Text(format!(r#"{{"error":"transcode_unavailable","details":"{}"}}"#, e))).await;
                let _ = ws_tx.send(Message::Close(None)).await;
                return Ok(());
            }
        };

        let mut rx = worker.subscribe();

        // Relay loop for transcoded H.264 stream
        loop {
            tokio::select! {
                frame_res = rx.recv() => {
                    match frame_res {
                        Ok(frame) => {
                            if let Some(buf) = frame.host_buffer() {
                                let is_key = (frame.format & FLAG_KEYFRAME) != 0;
                                let wsp_bytes = pack_wsp_frame(is_key, frame.timestamp_us, buf);
                                if ws_tx.send(Message::Binary(wsp_bytes.into())).await.is_err() {
                                    break;
                                }
                            }
                        }
                        Err(broadcast::error::RecvError::Lagged(_)) => continue,
                        Err(broadcast::error::RecvError::Closed) => break,
                    }
                }
                msg = ws_rx.next() => {
                    match msg {
                        Some(Ok(Message::Close(_))) | None => break,
                        _ => {}
                    }
                }
            }
        }

        worker.unsubscribe();
        println!("🛑 [Hydra WSP] Client {} disconnected from transcode '{}'", peer_addr, stream_id);
        return Ok(());
    }

    // Branch 2: Passthrough Zero-Copy stream (Native H.264 or H.265 raw)
    let (mut session, initial_keyframe) = match stream_entry.gateway.subscribe(BIT_WEBSOCKET) {
        Ok(sub) => sub,
        Err(e) => {
            let _ = ws_tx.send(Message::Text(format!(r#"{{"error":"subscription_failed","details":"{}"}}"#, e))).await;
            let _ = ws_tx.send(Message::Close(None)).await;
            return Ok(());
        }
    };

    // Instant Playback: Send cached keyframe immediately
    if let Some(kf) = initial_keyframe {
        if let Some(buf) = kf.host_buffer() {
            let wsp_bytes = pack_wsp_frame(true, kf.timestamp_us, buf);
            let _ = ws_tx.send(Message::Binary(wsp_bytes.into())).await;
        }
    }

    // Stream loop
    loop {
        tokio::select! {
            frame_res = session.recv_frame() => {
                match frame_res {
                    Ok(frame) => {
                        if let Some(buf) = frame.host_buffer() {
                            let is_key = (frame.format & FLAG_KEYFRAME) != 0;
                            let wsp_bytes = pack_wsp_frame(is_key, frame.timestamp_us, buf);
                            if ws_tx.send(Message::Binary(wsp_bytes.into())).await.is_err() {
                                break;
                            }
                        }
                    }
                    Err(_) => break,
                }
            }
            msg = ws_rx.next() => {
                match msg {
                    Some(Ok(Message::Close(_))) | None => break,
                    _ => {}
                }
            }
        }
    }

    println!("🛑 [Hydra WSP] Client {} disconnected from '{}'", peer_addr, stream_id);
    Ok(())
}
