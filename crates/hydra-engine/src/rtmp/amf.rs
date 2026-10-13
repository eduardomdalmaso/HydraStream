//! Minimal AMF0 command and data types codec for RTMP signaling

use std::sync::Arc;
use tokio::io::AsyncWrite;

use crate::gateway::AsyncStreamGateway;
use crate::server::{StreamCodec, StreamRegistry};
use crate::shm::ShmWriter;

use super::chunk::*;

pub fn read_amf0_string(data: &mut &[u8]) -> Option<String> {
    if data.is_empty() || data[0] != 0x02 {
        return None;
    }
    *data = &data[1..];
    if data.len() < 2 {
        return None;
    }
    let len = u16::from_be_bytes(data[0..2].try_into().unwrap()) as usize;
    *data = &data[2..];
    if data.len() < len {
        return None;
    }
    let s = String::from_utf8_lossy(&data[0..len]).to_string();
    *data = &data[len..];
    Some(s)
}

pub fn read_amf0_number(data: &mut &[u8]) -> Option<f64> {
    if data.is_empty() || data[0] != 0x00 {
        return None;
    }
    *data = &data[1..];
    if data.len() < 8 {
        return None;
    }
    let val = f64::from_be_bytes(data[0..8].try_into().unwrap());
    *data = &data[8..];
    Some(val)
}

pub fn skip_amf0_value(data: &mut &[u8]) -> bool {
    if data.is_empty() {
        return false;
    }
    let marker = data[0];
    *data = &data[1..];
    match marker {
        0x00 => {
            if data.len() < 8 {
                return false;
            }
            *data = &data[8..];
            true
        }
        0x01 => {
            if data.is_empty() {
                return false;
            }
            *data = &data[1..];
            true
        }
        0x02 => {
            if data.len() < 2 {
                return false;
            }
            let len = u16::from_be_bytes(data[0..2].try_into().unwrap()) as usize;
            *data = &data[2..];
            if data.len() < len {
                return false;
            }
            *data = &data[len..];
            true
        }
        0x05 => true,
        _ => false,
    }
}

pub fn write_amf0_string(buf: &mut Vec<u8>, s: &str) {
    buf.push(0x02);
    buf.extend_from_slice(&(s.len() as u16).to_be_bytes());
    buf.extend_from_slice(s.as_bytes());
}

pub fn write_amf0_number(buf: &mut Vec<u8>, val: f64) {
    buf.push(0x00);
    buf.extend_from_slice(&val.to_be_bytes());
}

pub fn write_amf0_prop_string(buf: &mut Vec<u8>, key: &str, val: &str) {
    buf.extend_from_slice(&(key.len() as u16).to_be_bytes());
    buf.extend_from_slice(key.as_bytes());
    write_amf0_string(buf, val);
}

pub fn write_amf0_prop_number(buf: &mut Vec<u8>, key: &str, val: f64) {
    buf.extend_from_slice(&(key.len() as u16).to_be_bytes());
    buf.extend_from_slice(key.as_bytes());
    write_amf0_number(buf, val);
}

pub async fn handle_amf0_command<S: AsyncWrite + Unpin>(
    stream: &mut S,
    state: &mut super::server::RtmpSessionState,
    msg: &RtmpMessage,
    registry: &Arc<StreamRegistry>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let mut payload = &msg.payload[..];
    let cmd_name = match read_amf0_string(&mut payload) {
        Some(s) => s,
        None => return Ok(()),
    };
    let trans_id = read_amf0_number(&mut payload).unwrap_or(0.0);

    match cmd_name.as_str() {
        "connect" => {
            write_rtmp_message(stream, 2, MSG_WINDOW_ACK_SIZE, 0, 0, &2_500_000u32.to_be_bytes(), RTMP_SERVER_OUT_CHUNK_SIZE).await?;
            let mut peer_bw = Vec::with_capacity(5);
            peer_bw.extend_from_slice(&2_500_000u32.to_be_bytes());
            peer_bw.push(2u8);
            write_rtmp_message(stream, 2, MSG_SET_PEER_BANDWIDTH, 0, 0, &peer_bw, RTMP_SERVER_OUT_CHUNK_SIZE).await?;

            state.out_chunk_size = RTMP_SERVER_OUT_CHUNK_SIZE;
            write_rtmp_message(stream, 2, MSG_SET_CHUNK_SIZE, 0, 0, &(state.out_chunk_size as u32).to_be_bytes(), RTMP_SERVER_OUT_CHUNK_SIZE).await?;

            let mut body = Vec::new();
            write_amf0_string(&mut body, "_result");
            write_amf0_number(&mut body, trans_id);

            body.push(0x03);
            write_amf0_prop_string(&mut body, "fmsVer", "FMS/3,0,1,123");
            write_amf0_prop_number(&mut body, "capabilities", 31.0);
            body.extend_from_slice(&[0x00, 0x00, 0x09]);

            body.push(0x03);
            write_amf0_prop_string(&mut body, "level", "status");
            write_amf0_prop_string(&mut body, "code", "NetConnection.Connect.Success");
            write_amf0_prop_string(&mut body, "description", "Connection succeeded");
            body.extend_from_slice(&[0x00, 0x00, 0x09]);

            write_rtmp_message(stream, 3, MSG_AMF0_COMMAND, 0, 0, &body, state.out_chunk_size).await?;
        }
        "releaseStream" | "FCPublish" => {
            let mut body = Vec::new();
            write_amf0_string(&mut body, "_result");
            write_amf0_number(&mut body, trans_id);
            body.push(0x05);
            body.push(0x05);
            write_rtmp_message(stream, 3, MSG_AMF0_COMMAND, 0, 0, &body, state.out_chunk_size).await?;
        }
        "createStream" => {
            let mut body = Vec::new();
            write_amf0_string(&mut body, "_result");
            write_amf0_number(&mut body, trans_id);
            body.push(0x05);
            write_amf0_number(&mut body, 1.0);
            write_rtmp_message(stream, 3, MSG_AMF0_COMMAND, 0, 0, &body, state.out_chunk_size).await?;
        }
        "publish" => {
            let _null = skip_amf0_value(&mut payload);
            let publish_path = read_amf0_string(&mut payload).unwrap_or_else(|| "live".to_string());
            let clean_stream_id = publish_path
                .split('?')
                .next()
                .unwrap_or("live")
                .trim_matches('/')
                .to_string();

            state.stream_id = Some(clean_stream_id.clone());

            match ShmWriter::create(&clean_stream_id, 1920, 1080, 1, 16) {
                Ok(w) => state.shm_writer = Some(w),
                Err(e) => eprintln!("⚠️ [Hydra RTMP] SHM writer init warning: {}", e),
            }

            let hub = Arc::new(crate::transport::BroadcastHub::new(16));
            let gateway = Arc::new(AsyncStreamGateway::new(hub, 32));
            registry.register(&clean_stream_id, StreamCodec::H264, Arc::clone(&gateway));
            state.gateway = Some(gateway);

            println!("📡 [Hydra RTMP] Stream published successfully: '{}'", clean_stream_id);

            let mut body = Vec::new();
            write_amf0_string(&mut body, "onStatus");
            write_amf0_number(&mut body, 0.0);
            body.push(0x05);
            body.push(0x03);
            write_amf0_prop_string(&mut body, "level", "status");
            write_amf0_prop_string(&mut body, "code", "NetStream.Publish.Start");
            write_amf0_prop_string(&mut body, "description", &format!("Publishing '{}'", clean_stream_id));
            body.extend_from_slice(&[0x00, 0x00, 0x09]);

            write_rtmp_message(stream, 3, MSG_AMF0_COMMAND, 0, 1, &body, state.out_chunk_size).await?;
        }
        _ => {}
    }

    Ok(())
}
