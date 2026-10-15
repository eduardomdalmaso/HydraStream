use std::process::{Command, Stdio};
use std::io::Write;
use futures_util::StreamExt;
use url::Url;

pub async fn capture_snapshot_native(raw_url: &str, timeout_secs: u64) -> Result<Vec<u8>, Box<dyn std::error::Error + Send + Sync>> {
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

    let describe_future = retina::client::Session::describe(url, session_options);
    let mut session = tokio::time::timeout(std::time::Duration::from_secs(timeout_secs), describe_future)
        .await
        .map_err(|_| "Timeout connecting and describing RTSP stream")??;

    let mut video_i = None;
    let mut codec_name = "h265";
    for (i, stream) in session.streams().iter().enumerate() {
        if stream.media() == "video" {
            video_i = Some(i);
            let c = format!("{:?}", stream.encoding_name()).to_lowercase();
            if c.contains("h264") {
                codec_name = "h264";
            }
            break;
        }
    }

    let video_i = video_i.ok_or("No video stream found in SDP presentation")?;

    let setup_options = retina::client::SetupOptions::default()
        .frame_format(retina::codec::FrameFormat::SIMPLE);

    session.setup(video_i, setup_options).await?;

    let playing = session.play(retina::client::PlayOptions::default()).await?;
    let mut demuxed = playing.demuxed()?;

    let mut video_buffer: Vec<u8> = Vec::new();
    let mut found_keyframe = false;
    let max_frames = 120; // up to 4-5 seconds at 25 fps
    let mut frame_count = 0;

    let loop_future = async {
        while let Some(item_res) = demuxed.next().await {
            if let Ok(retina::codec::CodecItem::VideoFrame(frame)) = item_res {
                frame_count += 1;
                if frame.is_random_access_point() {
                    found_keyframe = true;
                }
                if found_keyframe {
                    video_buffer.extend_from_slice(frame.data());
                    // 2-3 frames are plenty for FFmpeg to reconstruct a pristine keyframe
                    if frame_count >= 3 {
                        break;
                    }
                }
            }
            if frame_count >= max_frames {
                break;
            }
        }
    };

    tokio::time::timeout(std::time::Duration::from_secs(timeout_secs), loop_future)
        .await
        .map_err(|_| "Timeout waiting for video keyframe")?;

    if video_buffer.is_empty() {
        return Err("No video frame data captured from stream".into());
    }

    // Decode single keyframe JPEG via local FFmpeg stdin
    let input_format = if codec_name == "h264" { "h264" } else { "hevc" };
    let mut child = Command::new("ffmpeg")
        .args(&[
            "-loglevel", "error",
            "-f", input_format,
            "-i", "-",
            "-vframes", "1",
            "-q:v", "3",
            "-f", "image2",
            "-",
        ])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()?;

    if let Some(mut stdin) = child.stdin.take() {
        let _ = stdin.write_all(&video_buffer);
        let _ = stdin.flush();
    }

    let output = child.wait_with_output()?;
    if output.status.success() && !output.stdout.is_empty() {
        Ok(output.stdout)
    } else {
        Err("FFmpeg failed to decode captured keyframe to JPEG".into())
    }
}
