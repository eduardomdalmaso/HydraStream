//! Ingestão RTSP com suporte a Digest Auth e forward de pacotes RTP em Rust

use std::error::Error;
use bytes::Bytes;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;
use tokio::sync::broadcast;
use url::Url;

pub async fn run_relay_ingest(
    source_url: String,
    tx: broadcast::Sender<Bytes>,
    sdp_out: tokio::sync::oneshot::Sender<String>,
) -> Result<(), Box<dyn Error + Send + Sync>> {
    let parsed = Url::parse(&source_url)?;
    let host = parsed.host_str().ok_or("Missing host")?;
    let port = parsed.port().unwrap_or(554);
    let username = parsed.username().to_string();
    let password = parsed.password().unwrap_or("").to_string();

    let target_addr = format!("{}:{}", host, port);
    println!("🔄 [Relay Ingest] Connecting to source at {}", target_addr);

    let mut stream = TcpStream::connect(&target_addr).await?;
    let uri = format!("rtsp://{}:{}{}", host, port, parsed.path());

    // 1. DESCRIBE inicial
    let req1 = format!("DESCRIBE {} RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: HydraStream\r\n\r\n", uri);
    stream.write_all(req1.as_bytes()).await?;

    let mut buf = vec![0u8; 8192];
    let n = stream.read(&mut buf).await?;
    let resp1 = String::from_utf8_lossy(&buf[..n]);

    let mut auth_hdr = String::new();
    if resp1.contains("401 Unauthorized") {
        let realm = extract_param(&resp1, "realm=\"", "\"");
        let nonce = extract_param(&resp1, "nonce=\"", "\"");

        if !realm.is_empty() && !nonce.is_empty() {
            let ha1 = format!("{:x}", md5::compute(format!("{}:{}:{}", username, realm, password)));
            let ha2 = format!("{:x}", md5::compute(format!("DESCRIBE:{}", uri)));
            let resp_hash = format!("{:x}", md5::compute(format!("{}:{}:{}", ha1, nonce, ha2)));
            auth_hdr = format!(
                "Digest username=\"{}\", realm=\"{}\", nonce=\"{}\", uri=\"{}\", response=\"{}\"",
                username, realm, nonce, uri, resp_hash
            );
        }
    }

    // 2. DESCRIBE com Auth
    let req2 = if !auth_hdr.is_empty() {
        format!("DESCRIBE {} RTSP/1.0\r\nCSeq: 2\r\nUser-Agent: HydraStream\r\nAuthorization: {}\r\n\r\n", uri, auth_hdr)
    } else {
        format!("DESCRIBE {} RTSP/1.0\r\nCSeq: 2\r\nUser-Agent: HydraStream\r\n\r\n", uri)
    };
    stream.write_all(req2.as_bytes()).await?;

    let n = stream.read(&mut buf).await?;
    let resp2 = String::from_utf8_lossy(&buf[..n]);

    let sdp = if let Some(pos) = resp2.find("\r\n\r\n") {
        resp2[pos + 4..].to_string()
    } else {
        "".to_string()
    };

    let _ = sdp_out.send(sdp);

    // 3. SETUP trackID=video
    let track_uri = format!("{}/trackID=video", uri.trim_end_matches('/'));
    let req_setup = format!(
        "SETUP {} RTSP/1.0\r\nCSeq: 3\r\nUser-Agent: HydraStream\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n\r\n",
        track_uri
    );
    stream.write_all(req_setup.as_bytes()).await?;
    let n = stream.read(&mut buf).await?;
    let resp3 = String::from_utf8_lossy(&buf[..n]);

    let session = extract_session(&resp3);

    // 4. PLAY
    let req_play = format!(
        "PLAY {} RTSP/1.0\r\nCSeq: 4\r\nUser-Agent: HydraStream\r\nSession: {}\r\nRange: npt=0.000-\r\n\r\n",
        uri, session
    );
    stream.write_all(req_play.as_bytes()).await?;
    let _ = stream.read(&mut buf).await?;

    println!("✅ [Relay Ingest] Video stream negotiated successfully. Forwarding RTP packets...");

    // 5. Loop de leitura de pacotes RTP e broadcast para os clientes downstream
    let mut rtp_buf = vec![0u8; 65536];
    loop {
        let n = stream.read(&mut rtp_buf).await?;
        if n == 0 {
            break;
        }
        let pkt = Bytes::copy_from_slice(&rtp_buf[..n]);
        let _ = tx.send(pkt);
    }

    Ok(())
}

fn extract_param(text: &str, start_tag: &str, end_tag: &str) -> String {
    if let Some(start) = text.find(start_tag) {
        let s = &text[start + start_tag.len()..];
        if let Some(end) = s.find(end_tag) {
            return s[..end].to_string();
        }
    }
    "".to_string()
}

fn extract_session(resp: &str) -> String {
    for line in resp.lines() {
        if line.to_ascii_lowercase().starts_with("session:") {
            let val = line["session:".len()..].trim();
            if let Some(sc) = val.find(';') {
                return val[..sc].trim().to_string();
            }
            return val.to_string();
        }
    }
    "1".to_string()
}
