//! RTMP chunk stream reader and reassembly state machine

use std::collections::HashMap;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};

pub const RTMP_DEFAULT_CHUNK_SIZE: usize = 128;
pub const RTMP_SERVER_OUT_CHUNK_SIZE: usize = 4096;

pub const MSG_SET_CHUNK_SIZE: u8 = 1;
pub const MSG_WINDOW_ACK_SIZE: u8 = 5;
pub const MSG_SET_PEER_BANDWIDTH: u8 = 6;
pub const MSG_AUDIO: u8 = 8;
pub const MSG_VIDEO: u8 = 9;
pub const MSG_AMF0_COMMAND: u8 = 20;

#[derive(Default)]
pub struct ChunkStreamState {
    pub msg_type_id: u8,
    pub msg_length: usize,
    pub timestamp: u32,
    pub msg_stream_id: u32,
    pub data: Vec<u8>,
}

pub struct RtmpMessage {
    pub csid: u32,
    pub msg_type_id: u8,
    pub timestamp: u32,
    pub msg_stream_id: u32,
    pub payload: Vec<u8>,
}

pub async fn read_u24<S: AsyncRead + Unpin>(stream: &mut S) -> std::io::Result<u32> {
    let mut b = [0u8; 3];
    stream.read_exact(&mut b).await?;
    Ok(((b[0] as u32) << 16) | ((b[1] as u32) << 8) | (b[2] as u32))
}

pub async fn read_rtmp_message<S: AsyncRead + Unpin>(
    stream: &mut S,
    chunk_states: &mut HashMap<u32, ChunkStreamState>,
    in_chunk_size: usize,
) -> Result<Option<RtmpMessage>, Box<dyn std::error::Error + Send + Sync>> {
    loop {
        let first_byte = match stream.read_u8().await {
            Ok(b) => b,
            Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
            Err(e) => return Err(e.into()),
        };

        let fmt = first_byte >> 6;
        let mut csid = (first_byte & 0x3F) as u32;

        if csid == 0 {
            csid = 64 + (stream.read_u8().await? as u32);
        } else if csid == 1 {
            let b1 = stream.read_u8().await? as u32;
            let b2 = stream.read_u8().await? as u32;
            csid = 64 + b1 + (b2 * 256);
        }

        let chunk_state = chunk_states.entry(csid).or_default();

        match fmt {
            0 => {
                let ts = read_u24(stream).await?;
                let length = read_u24(stream).await? as usize;
                let msg_type_id = stream.read_u8().await?;
                let msg_stream_id = stream.read_u32_le().await?;

                let timestamp = if ts == 0xFFFFFF {
                    stream.read_u32().await?
                } else {
                    ts
                };

                chunk_state.timestamp = timestamp;
                chunk_state.msg_length = length;
                chunk_state.msg_type_id = msg_type_id;
                chunk_state.msg_stream_id = msg_stream_id;
                chunk_state.data.clear();
            }
            1 => {
                let ts_delta = read_u24(stream).await?;
                let length = read_u24(stream).await? as usize;
                let msg_type_id = stream.read_u8().await?;

                let timestamp_delta = if ts_delta == 0xFFFFFF {
                    stream.read_u32().await?
                } else {
                    ts_delta
                };

                chunk_state.timestamp = chunk_state.timestamp.wrapping_add(timestamp_delta);
                chunk_state.msg_length = length;
                chunk_state.msg_type_id = msg_type_id;
                chunk_state.data.clear();
            }
            2 => {
                let ts_delta = read_u24(stream).await?;
                let timestamp_delta = if ts_delta == 0xFFFFFF {
                    stream.read_u32().await?
                } else {
                    ts_delta
                };
                chunk_state.timestamp = chunk_state.timestamp.wrapping_add(timestamp_delta);
                chunk_state.data.clear();
            }
            3 => {}
            _ => unreachable!(),
        }

        let needed = chunk_state.msg_length.saturating_sub(chunk_state.data.len());
        let to_read = std::cmp::min(needed, in_chunk_size);

        if to_read > 0 {
            let mut chunk_buf = vec![0u8; to_read];
            stream.read_exact(&mut chunk_buf).await?;
            chunk_state.data.extend_from_slice(&chunk_buf);
        }

        if chunk_state.data.len() >= chunk_state.msg_length {
            let msg = RtmpMessage {
                csid,
                msg_type_id: chunk_state.msg_type_id,
                timestamp: chunk_state.timestamp,
                msg_stream_id: chunk_state.msg_stream_id,
                payload: std::mem::take(&mut chunk_state.data),
            };
            return Ok(Some(msg));
        }
    }
}

pub async fn write_rtmp_message<S: AsyncWrite + Unpin>(
    stream: &mut S,
    csid: u8,
    msg_type: u8,
    timestamp: u32,
    msg_stream_id: u32,
    payload: &[u8],
    chunk_size: usize,
) -> std::io::Result<()> {
    let mut chunks = payload.chunks(chunk_size);
    if let Some(first_chunk) = chunks.next() {
        let mut header = Vec::with_capacity(12 + first_chunk.len());
        header.push(csid & 0x3F);

        header.push(((timestamp >> 16) & 0xFF) as u8);
        header.push(((timestamp >> 8) & 0xFF) as u8);
        header.push((timestamp & 0xFF) as u8);

        let len = payload.len();
        header.push(((len >> 16) & 0xFF) as u8);
        header.push(((len >> 8) & 0xFF) as u8);
        header.push((len & 0xFF) as u8);

        header.push(msg_type);
        header.extend_from_slice(&msg_stream_id.to_le_bytes());
        header.extend_from_slice(first_chunk);
        stream.write_all(&header).await?;
    }

    for chunk in chunks {
        let mut header = Vec::with_capacity(1 + chunk.len());
        header.push(0xC0 | (csid & 0x3F));
        header.extend_from_slice(chunk);
        stream.write_all(&header).await?;
    }
    stream.flush().await?;
    Ok(())
}
