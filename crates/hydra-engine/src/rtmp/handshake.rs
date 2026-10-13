//! RTMP standard handshake implementation (C0/S0 + C1/S1 + C2/S2)

use std::time::Instant;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};

/// Perform standard RTMP handshake with client
pub async fn rtmp_handshake<S: AsyncRead + AsyncWrite + Unpin>(
    stream: &mut S,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    // 1. Read C0 (1 byte: version 3)
    let c0 = stream.read_u8().await?;
    if c0 != 3 {
        return Err(format!("Unsupported RTMP version: {}", c0).into());
    }

    // 2. Read C1 (1536 bytes: 4 bytes time + 4 bytes zero + 1528 random)
    let mut c1 = vec![0u8; 1536];
    stream.read_exact(&mut c1).await?;

    // 3. Prepare S0 + S1 + S2
    let mut response = Vec::with_capacity(1 + 1536 + 1536);
    response.push(3u8); // S0

    let mut s1 = vec![0u8; 1536];
    let now_ms = (Instant::now().elapsed().as_millis() as u32).to_be_bytes();
    s1[0..4].copy_from_slice(&now_ms);
    for i in 8..1536 {
        s1[i] = (i & 0xFF) as u8;
    }
    response.extend_from_slice(&s1);

    // S2 = C1 echo
    response.extend_from_slice(&c1);

    // Write S0 + S1 + S2
    stream.write_all(&response).await?;
    stream.flush().await?;

    // 4. Read C2 (1536 bytes: echo of S1)
    let mut c2 = vec![0u8; 1536];
    stream.read_exact(&mut c2).await?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_rtmp_handshake_mock_loopback() {
        let (mut client, mut server) = tokio::io::duplex(4096);

        let server_task = tokio::spawn(async move {
            rtmp_handshake(&mut server).await
        });

        let client_task = tokio::spawn(async move {
            client.write_u8(3).await?;
            let c1 = vec![0x42u8; 1536];
            client.write_all(&c1).await?;
            client.flush().await?;

            let s0 = client.read_u8().await?;
            assert_eq!(s0, 3);
            let mut s1 = vec![0u8; 1536];
            client.read_exact(&mut s1).await?;
            let mut s2 = vec![0u8; 1536];
            client.read_exact(&mut s2).await?;
            assert_eq!(s2, c1);

            client.write_all(&s1).await?;
            client.flush().await?;

            Ok::<(), Box<dyn std::error::Error + Send + Sync>>(())
        });

        let (srv_res, cli_res) = tokio::join!(server_task, client_task);
        assert!(srv_res.unwrap().is_ok());
        assert!(cli_res.unwrap().is_ok());
    }
}
