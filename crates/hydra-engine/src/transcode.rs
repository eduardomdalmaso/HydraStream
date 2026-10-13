//! Dynamic Hardware-Accelerated Video Transcoder Pool
//! Uses FFmpeg with NVENC/CUDA on any detected NVIDIA GPU.
//! Strict rule: Exclusively uses NVIDIA GPU; fails with ErrNvidiaGpuRequired if no GPU is available.

use std::collections::HashMap;
use std::process::Stdio;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::Arc;
use parking_lot::RwLock;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::process::{ChildStdin, Command};
use tokio::sync::broadcast;
use crate::codec::NalParser;
use crate::transport::FramePayload;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TranscodeError {
    NvidiaGpuRequired,
    ProcessSpawnFailed(String),
    StreamNotFound(String),
    TranscodeFailed(String),
}

impl std::fmt::Display for TranscodeError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::NvidiaGpuRequired => write!(f, "NVIDIA GPU with NVENC/CUDA is required for H.265 transcoding"),
            Self::ProcessSpawnFailed(e) => write!(f, "Failed to spawn FFmpeg NVENC worker: {}", e),
            Self::StreamNotFound(id) => write!(f, "Stream '{}' not found in transcode registry", id),
            Self::TranscodeFailed(e) => write!(f, "Transcoding failure: {}", e),
        }
    }
}

impl std::error::Error for TranscodeError {}

/// Dynamically detects if any NVIDIA GPU is accessible on the system.
/// Inspects `nvidia-smi` or `/dev/nvidiactl` without hardcoding any specific model.
pub fn detect_nvidia_gpu() -> Option<String> {
    // 1. Fast check for Linux NVIDIA device nodes
    #[cfg(target_os = "linux")]
    {
        if !std::path::Path::new("/dev/nvidiactl").exists() && !std::path::Path::new("/dev/nvidia0").exists() {
            return None;
        }
    }

    // 2. Query GPU info dynamically via nvidia-smi
    let output = std::process::Command::new("nvidia-smi")
        .args(["--query-gpu=name,index", "--format=csv,noheader"])
        .output()
        .ok()?;

    if !output.status.success() {
        return None;
    }

    let text = String::from_utf8_lossy(&output.stdout);
    let first_line = text.lines().next()?.trim();
    if first_line.is_empty() {
        None
    } else {
        Some(first_line.to_string())
    }
}

/// Locates local bin/ffmpeg or falls back to system PATH
pub fn find_ffmpeg_bin() -> String {
    for candidate in &["bin/ffmpeg", "./bin/ffmpeg", "bin/ffmpeg.exe"] {
        if std::path::Path::new(candidate).exists() {
            return candidate.to_string();
        }
    }
    "ffmpeg".to_string()
}

/// A running FFmpeg NVENC transcode instance for a single stream
pub struct TranscodeWorker {
    pub stream_id: String,
    subscriber_count: Arc<AtomicUsize>,
    tx_frames: broadcast::Sender<FramePayload>,
    is_running: Arc<AtomicBool>,
    child_stdin: Arc<tokio::sync::Mutex<Option<ChildStdin>>>,
}

impl TranscodeWorker {
    pub fn new(stream_id: &str) -> Result<Self, TranscodeError> {
        let gpu_info = detect_nvidia_gpu().ok_or(TranscodeError::NvidiaGpuRequired)?;
        println!(
            "🚀 [Hydra Transcoder] Spawning NVENC worker for '{}' on detected GPU: {}",
            stream_id, gpu_info
        );

        let ffmpeg_bin = find_ffmpeg_bin();
        let mut cmd = Command::new(ffmpeg_bin);
        cmd.args([
            "-nostdin",
            "-loglevel", "error",
            "-hwaccel", "cuda",
            "-c:v", "hevc_cuvid",
            "-i", "pipe:0",
            "-c:v", "h264_nvenc",
            "-preset", "p1",
            "-tune", "ll",
            "-zerolatency", "1",
            "-g", "30",
            "-b:v", "2000k",
            "-maxrate", "2500k",
            "-bufsize", "500k",
            "-f", "h264",
            "pipe:1",
        ])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());

        let mut child = cmd.spawn().map_err(|e| {
            TranscodeError::ProcessSpawnFailed(format!("{}: {}", e, stream_id))
        })?;

        let stdin = child.stdin.take().ok_or_else(|| {
            TranscodeError::ProcessSpawnFailed("Failed to capture FFmpeg stdin".into())
        })?;

        let stdout = child.stdout.take().ok_or_else(|| {
            TranscodeError::ProcessSpawnFailed("Failed to capture FFmpeg stdout".into())
        })?;

        let (tx, _) = broadcast::channel(64);
        let subscriber_count = Arc::new(AtomicUsize::new(0));
        let is_running = Arc::new(AtomicBool::new(true));

        let tx_clone = tx.clone();
        let running_clone = is_running.clone();
        let stream_id_clone = stream_id.to_string();

        // Background reader task: reads H.264 NALUs from FFmpeg stdout and publishes to broadcast channel
        tokio::spawn(async move {
            Self::reader_loop(stdout, tx_clone, running_clone, stream_id_clone).await;
        });

        // Watchdog task to reap child process
        let running_watch = is_running.clone();
        tokio::spawn(async move {
            let _ = child.wait().await;
            running_watch.store(false, Ordering::Release);
        });

        Ok(Self {
            stream_id: stream_id.to_string(),
            subscriber_count,
            tx_frames: tx,
            is_running,
            child_stdin: Arc::new(tokio::sync::Mutex::new(Some(stdin))),
        })
    }

    /// Feeds incoming raw H.265 NALU into FFmpeg stdin
    pub async fn feed_input_frame(&self, frame_data: &[u8]) -> Result<(), TranscodeError> {
        if !self.is_running.load(Ordering::Acquire) {
            return Err(TranscodeError::TranscodeFailed("FFmpeg process is not running".into()));
        }

        let mut lock = self.child_stdin.lock().await;
        if let Some(stdin) = lock.as_mut() {
            if let Err(e) = stdin.write_all(frame_data).await {
                return Err(TranscodeError::TranscodeFailed(format!("Write to FFmpeg stdin failed: {}", e)));
            }
            let _ = stdin.flush().await;
        }
        Ok(())
    }

    pub fn subscribe(&self) -> broadcast::Receiver<FramePayload> {
        self.subscriber_count.fetch_add(1, Ordering::Relaxed);
        self.tx_frames.subscribe()
    }

    pub fn unsubscribe(&self) -> usize {
        let prev = self.subscriber_count.fetch_sub(1, Ordering::Relaxed);
        prev.saturating_sub(1)
    }

    pub fn active_subscribers(&self) -> usize {
        self.subscriber_count.load(Ordering::Relaxed)
    }

    pub fn is_alive(&self) -> bool {
        self.is_running.load(Ordering::Acquire)
    }

    pub async fn stop(&self) {
        self.is_running.store(false, Ordering::Release);
        let mut lock = self.child_stdin.lock().await;
        *lock = None; // Dropping stdin causes FFmpeg to exit cleanly on EOF
    }

    async fn reader_loop(
        mut stdout: tokio::process::ChildStdout,
        tx: broadcast::Sender<FramePayload>,
        running: Arc<AtomicBool>,
        stream_id: String,
    ) {
        let mut buf = vec![0u8; 65536];
        let mut stream_accumulator = Vec::with_capacity(131072);
        let mut frame_seq: u64 = 0;

        while running.load(Ordering::Acquire) {
            match stdout.read(&mut buf).await {
                Ok(0) => break, // EOF
                Ok(n) => {
                    stream_accumulator.extend_from_slice(&buf[..n]);

                    // Split Annex-B NAL units (delimited by 0x00000001 or 0x000001)
                    while let Some(pos) = find_nal_boundary(&stream_accumulator[4..]) {
                        let split_idx = pos + 4;
                        let nalu = stream_accumulator[..split_idx].to_vec();
                        stream_accumulator.drain(..split_idx);

                        let flags = NalParser::classify_frame_flags(&nalu, false); // H.264
                        frame_seq += 1;
                        let ts_now = std::time::SystemTime::now()
                            .duration_since(std::time::UNIX_EPOCH)
                            .map(|d| d.as_micros() as u64)
                            .unwrap_or(0);

                        let payload = FramePayload::new_host(
                            frame_seq,
                            ts_now,
                            1920,
                            1080,
                            flags,
                            crate::transport::BIT_WEBSOCKET,
                            nalu,
                        );

                        let _ = tx.send(payload);
                    }
                }
                Err(_) => break,
            }
        }

        println!("🛑 [Hydra Transcoder] Output reader closed for '{}'", stream_id);
    }
}

fn find_nal_boundary(data: &[u8]) -> Option<usize> {
    if data.len() < 4 {
        return None;
    }
    for i in 0..data.len() - 3 {
        if data[i] == 0 && data[i + 1] == 0 && (data[i + 2] == 1 || (data[i + 2] == 0 && data[i + 3] == 1)) {
            return Some(i);
        }
    }
    None
}

/// Dynamic manager for lazy reference-counted transcoding workers
pub struct TranscodePool {
    workers: Arc<RwLock<HashMap<String, Arc<TranscodeWorker>>>>,
}

impl TranscodePool {
    pub fn new() -> Self {
        Self {
            workers: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Gets or spawns an on-demand NVENC transcode worker for the specified stream
    pub fn get_or_spawn(&self, stream_id: &str) -> Result<Arc<TranscodeWorker>, TranscodeError> {
        {
            let lock = self.workers.read();
            if let Some(w) = lock.get(stream_id) {
                if w.is_alive() {
                    return Ok(w.clone());
                }
            }
        }

        let worker = Arc::new(TranscodeWorker::new(stream_id)?);
        let mut lock = self.workers.write();
        lock.insert(stream_id.to_string(), worker.clone());
        Ok(worker)
    }

    /// Schedule cleanup of idle workers
    pub fn reap_idle_workers(&self) {
        let mut lock = self.workers.write();
        lock.retain(|id, w| {
            if !w.is_alive() || w.active_subscribers() == 0 {
                println!("🧹 [Hydra Transcoder] Reaping idle worker for '{}'", id);
                false
            } else {
                true
            }
        });
    }
}

impl Default for TranscodePool {
    fn default() -> Self {
        Self::new()
    }
}
