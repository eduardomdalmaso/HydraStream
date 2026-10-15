//! Native RTSP Egress Server & Relay in Rust
//! High-performance TCP interleaved RTP proxy for downstream VMS (Vezha, VLC, etc.)

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use bytes::Bytes;
use parking_lot::RwLock;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::broadcast;

pub struct StreamRelay {
    pub id: String,
    pub sdp: String,
    pub tx: broadcast::Sender<Bytes>,
}

#[derive(Clone)]
pub struct RtspRelayManager {
    streams: Arc<RwLock<HashMap<String, Arc<StreamRelay>>>>,
}

impl RtspRelayManager {
    pub fn new() -> Self {
        Self {
            streams: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    pub fn register(&self, id: String, sdp: String) -> broadcast::Sender<Bytes> {
        let (tx, _) = broadcast::channel(512);
        let relay = Arc::new(StreamRelay {
            id: id.clone(),
            sdp,
            tx: tx.clone(),
        });
        self.streams.write().insert(id, relay);
        tx
    }

    pub fn get(&self, id: &str) -> Option<Arc<StreamRelay>> {
        self.streams.read().get(id).cloned()
    }

    pub fn list(&self) -> Vec<String> {
        self.streams.read().keys().cloned().collect()
    }

    pub fn remove(&self, id: &str) {
        self.streams.write().remove(id);
    }
}

pub struct RtspServer {
    addr: SocketAddr,
    manager: RtspRelayManager,
}

impl RtspServer {
    pub fn new(addr: SocketAddr, manager: RtspRelayManager) -> Self {
        Self { addr, manager }
    }

    pub async fn run(&self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let listener = TcpListener::bind(self.addr).await?;
        println!("📡 [HydraStream Rust RTSP Server] Listening on rtsp://{}", self.addr);

        loop {
            let (socket, peer) = listener.accept().await?;
            let manager = self.manager.clone();
            tokio::spawn(async move {
                if let Err(e) = handle_client(socket, peer, manager).await {
                    eprintln!("⚠️ [RTSP Client {}] Session closed: {}", peer, e);
                }
            });
        }
    }
}

async fn handle_client(
    mut stream: TcpStream,
    peer: SocketAddr,
    manager: RtspRelayManager,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let mut buf = vec![0u8; 4096];
    let mut current_stream_id = String::new();
    let session_id = format!("{:x}", rand_u64());

    loop {
        let n = stream.read(&mut buf).await?;
        if n == 0 {
            break;
        }

        let req = String::from_utf8_lossy(&buf[..n]);
        let mut lines = req.lines();
        let first_line = lines.next().unwrap_or("");
        let mut parts = first_line.split_whitespace();
        let method = parts.next().unwrap_or("");
        let uri = parts.next().unwrap_or("");

        let mut cseq = "1";
        for line in req.lines() {
            if line.to_ascii_lowercase().starts_with("cseq:") {
                cseq = line["cseq:".len()..].trim();
            }
        }

        let stream_id = extract_stream_id(uri);
        if !stream_id.is_empty() {
            current_stream_id = stream_id;
        }

        match method {
            "OPTIONS" => {
                let resp = format!(
                    "RTSP/1.0 200 OK\r\nCSeq: {}\r\nPublic: OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY\r\n\r\n",
                    cseq
                );
                stream.write_all(resp.as_bytes()).await?;
            }
            "DESCRIBE" => {
                let relay = manager.get(&current_stream_id);
                if let Some(r) = relay {
                    let sdp = &r.sdp;
                    let resp = format!(
                        "RTSP/1.0 200 OK\r\nCSeq: {}\r\nContent-Type: application/sdp\r\nContent-Base: {}\r\nContent-Length: {}\r\n\r\n{}",
                        cseq, uri, sdp.len(), sdp
                    );
                    stream.write_all(resp.as_bytes()).await?;
                } else {
                    let resp = format!("RTSP/1.0 404 Not Found\r\nCSeq: {}\r\n\r\n", cseq);
                    stream.write_all(resp.as_bytes()).await?;
                }
            }
            "SETUP" => {
                let resp = format!(
                    "RTSP/1.0 200 OK\r\nCSeq: {}\r\nSession: {}\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n\r\n",
                    cseq, session_id
                );
                stream.write_all(resp.as_bytes()).await?;
            }
            "PLAY" => {
                let resp = format!(
                    "RTSP/1.0 200 OK\r\nCSeq: {}\r\nSession: {}\r\nRange: npt=0.000-\r\n\r\n",
                    cseq, session_id
                );
                stream.write_all(resp.as_bytes()).await?;

                if let Some(relay) = manager.get(&current_stream_id) {
                    println!("▶️ [RTSP Egress] Streaming '{}' to {}", current_stream_id, peer);
                    let mut rx = relay.tx.subscribe();
                    loop {
                        tokio::select! {
                            pkt_res = rx.recv() => {
                                match pkt_res {
                                    Ok(pkt) => {
                                        if let Err(e) = stream.write_all(&pkt).await {
                                            return Err(e.into());
                                        }
                                    }
                                    Err(broadcast::error::RecvError::Lagged(skipped)) => {
                                        eprintln!("⚠️ [RTSP Client {}] Lagged, skipped {} packets", peer, skipped);
                                    }
                                    Err(_) => break,
                                }
                            }
                            read_check = stream.read(&mut buf) => {
                                match read_check {
                                    Ok(0) | Err(_) => break,
                                    Ok(_) => {} // Client sent TEARDOWN or Keep-Alive
                                }
                            }
                        }
                    }
                }
                break;
            }
            "TEARDOWN" => {
                let resp = format!("RTSP/1.0 200 OK\r\nCSeq: {}\r\n\r\n", cseq);
                let _ = stream.write_all(resp.as_bytes()).await;
                break;
            }
            _ => {
                let resp = format!("RTSP/1.0 400 Bad Request\r\nCSeq: {}\r\n\r\n", cseq);
                stream.write_all(resp.as_bytes()).await?;
            }
        }
    }

    Ok(())
}

fn extract_stream_id(uri: &str) -> String {
    let clean = uri.trim_end_matches('/');
    if let Some(pos) = clean.rfind('/') {
        let seg = &clean[pos + 1..];
        if seg.starts_with("trackID=") {
            if let Some(prev) = clean[..pos].rfind('/') {
                return clean[prev + 1..pos].to_string();
            }
        }
        return seg.to_string();
    }
    "".to_string()
}

fn rand_u64() -> u64 {
    use std::time::SystemTime;
    SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64
}
