use futures_util::StreamExt;
use std::io::Write;
use url::Url;

pub async fn run_stream_pipe(raw_url: &str) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
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

    let mut session = retina::client::Session::describe(url, session_options).await?;
    let mut video_i = None;
    for (i, stream) in session.streams().iter().enumerate() {
        if stream.media() == "video" {
            video_i = Some(i);
            break;
        }
    }

    let video_i = video_i.ok_or("No video stream found")?;
    let setup_options = retina::client::SetupOptions::default()
        .frame_format(retina::codec::FrameFormat::SIMPLE);

    session.setup(video_i, setup_options).await?;
    let playing = session.play(retina::client::PlayOptions::default()).await?;
    let mut demuxed = playing.demuxed()?;

    let mut stdout = std::io::BufWriter::with_capacity(32768, std::io::stdout());
    let mut started = false;
    while let Some(item_res) = demuxed.next().await {
        if let Ok(retina::codec::CodecItem::VideoFrame(frame)) = item_res {
            if !started {
                if !frame.is_random_access_point() {
                    continue;
                }
                started = true;
            }
            if let Err(e) = stdout.write_all(frame.data()) {
                if e.kind() == std::io::ErrorKind::BrokenPipe {
                    break;
                }
            }
            let _ = stdout.flush();
        }
    }

    Ok(())
}
