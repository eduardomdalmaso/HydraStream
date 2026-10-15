use std::time::Instant;
use serde::{Deserialize, Serialize};
use url::Url;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RtspProbeResult {
    pub online: bool,
    pub codec: String,
    pub resolution: String,
    pub fps: u32,
    pub latency_ms: u64,
    pub sdp: String,
}

pub async fn probe_url(raw_url: &str) -> Result<RtspProbeResult, Box<dyn std::error::Error + Send + Sync>> {
    let start = Instant::now();
    let mut url = Url::parse(raw_url)?;

    let mut creds = None;
    if !url.username().is_empty() {
        let username = url.username().to_string();
        let password = url.password().unwrap_or("").to_string();
        creds = Some(retina::client::Credentials { username, password });
        let _ = url.set_username("");
        let _ = url.set_password(None);
    }

    let mut session_options = retina::client::SessionOptions::default();
    if creds.is_some() {
        session_options = session_options.creds(creds);
    }

    let session = retina::client::Session::describe(url, session_options).await?;
    let latency_ms = start.elapsed().as_millis() as u64;

    let mut codec = "unknown".to_string();
    let mut resolution = "1920x1080".to_string();
    let fps = 25;

    for stream in session.streams() {
        if stream.media() == "video" {
            let stream_codec = format!("{:?}", stream.encoding_name());
            if stream_codec.to_lowercase().contains("h265") || stream_codec.to_lowercase().contains("hevc") {
                codec = "h265".to_string();
            } else if stream_codec.to_lowercase().contains("h264") {
                codec = "h264".to_string();
            } else {
                codec = stream_codec;
            }
            if let Some(params) = stream.parameters() {
                let params_str = format!("{:?}", params);
                if params_str.contains("2560") && params_str.contains("1440") {
                    resolution = "2560x1440".to_string();
                } else if params_str.contains("3840") && params_str.contains("2160") {
                    resolution = "3840x2160".to_string();
                } else if params_str.contains("1280") && params_str.contains("720") {
                    resolution = "1280x720".to_string();
                } else if params_str.contains("640") && params_str.contains("360") {
                    resolution = "640x360".to_string();
                }
            }
            break;
        }
    }

    Ok(RtspProbeResult {
        online: true,
        codec,
        resolution,
        fps,
        latency_ms,
        sdp: "".to_string(),
    })
}
