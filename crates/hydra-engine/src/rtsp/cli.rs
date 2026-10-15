use std::io::Write;

pub fn run_probe(args: &[String]) {
    let mut url = String::new();
    let mut i = 0;
    while i < args.len() {
        if (args[i] == "--url" || args[i] == "-u") && i + 1 < args.len() {
            url = args[i + 1].clone();
            i += 1;
        }
        i += 1;
    }
    if url.is_empty() {
        eprintln!("Usage: hydra-engine probe --url <rtsp_url>");
        std::process::exit(1);
    }
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .expect("Failed to build Tokio runtime");
    rt.block_on(async {
        match crate::rtsp::probe_url(&url).await {
            Ok(res) => {
                let json = serde_json::to_string_pretty(&res).unwrap_or_default();
                println!("{}", json);
            }
            Err(e) => {
                eprintln!("Probe error: {}", e);
                std::process::exit(1);
            }
        }
    });
}

pub fn run_snapshot(args: &[String]) {
    let mut url = String::new();
    let mut out_path = String::new();
    let mut i = 0;
    while i < args.len() {
        if (args[i] == "--url" || args[i] == "-u") && i + 1 < args.len() {
            url = args[i + 1].clone();
            i += 1;
        } else if (args[i] == "--out" || args[i] == "-o") && i + 1 < args.len() {
            out_path = args[i + 1].clone();
            i += 1;
        }
        i += 1;
    }
    if url.is_empty() {
        eprintln!("Usage: hydra-engine snapshot --url <rtsp_url> [--out <output_path>]");
        std::process::exit(1);
    }
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .expect("Failed to build Tokio runtime");
    rt.block_on(async {
        match crate::rtsp::capture_snapshot_native(&url, 8).await {
            Ok(jpeg_bytes) => {
                if !out_path.is_empty() {
                    if let Some(parent) = std::path::Path::new(&out_path).parent() {
                        let _ = std::fs::create_dir_all(parent);
                    }
                    if let Err(e) = std::fs::write(&out_path, &jpeg_bytes) {
                        eprintln!("Failed to write snapshot to '{}': {}", out_path, e);
                        std::process::exit(1);
                    }
                    println!("Snapshot captured successfully ({} bytes) -> {}", jpeg_bytes.len(), out_path);
                } else {
                    let _ = std::io::stdout().write_all(&jpeg_bytes);
                }
            }
            Err(e) => {
                eprintln!("Snapshot error: {}", e);
                std::process::exit(1);
            }
        }
    });
}

pub fn run_stream(args: &[String]) {
    let mut url = String::new();
    let mut i = 0;
    while i < args.len() {
        if (args[i] == "--url" || args[i] == "-u") && i + 1 < args.len() {
            url = args[i + 1].clone();
            i += 1;
        }
        i += 1;
    }
    if url.is_empty() {
        eprintln!("Usage: hydra-engine stream --url <rtsp_url>");
        std::process::exit(1);
    }
    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .expect("Failed to build Tokio runtime");
    rt.block_on(async {
        if let Err(e) = crate::rtsp::run_stream_pipe(&url).await {
            eprintln!("Stream pipe error: {}", e);
            std::process::exit(1);
        }
    });
}

pub fn run_relay(args: &[String]) {
    let mut source_url = String::new();
    let mut stream_id = "cam101".to_string();
    let mut port: u16 = 8554;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--url" | "-u" | "--source" => {
                if i + 1 < args.len() {
                    source_url = args[i + 1].clone();
                    i += 1;
                }
            }
            "--id" => {
                if i + 1 < args.len() {
                    stream_id = args[i + 1].clone();
                    i += 1;
                }
            }
            "--port" | "-p" => {
                if i + 1 < args.len() {
                    port = args[i + 1].parse().unwrap_or(8554);
                    i += 1;
                }
            }
            _ => {}
        }
        i += 1;
    }

    if source_url.is_empty() {
        eprintln!("Usage: hydra-engine relay --source <rtsp_url> [--id <stream_id>] [--port 8554]");
        std::process::exit(1);
    }

    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .expect("Failed to build Tokio runtime for RTSP Relay");

    rt.block_on(async move {
        let addr = format!("0.0.0.0:{}", port).parse().expect("Invalid RTSP port");
        let manager = crate::rtsp::RtspRelayManager::new();
        let server = crate::rtsp::RtspServer::new(addr, manager.clone());

        let (sdp_tx, sdp_rx) = tokio::sync::oneshot::channel();
        let (pkt_tx, _) = tokio::sync::broadcast::channel(512);

        let ingest_url = source_url.clone();
        let p_tx = pkt_tx.clone();
        tokio::spawn(async move {
            if let Err(e) = crate::rtsp::run_relay_ingest(ingest_url, p_tx, sdp_tx).await {
                eprintln!("❌ [Relay Ingest Error] {}", e);
            }
        });

        match sdp_rx.await {
            Ok(sdp) => {
                let tx = manager.register(stream_id.clone(), sdp);
                let mut fwd_rx = pkt_tx.subscribe();
                tokio::spawn(async move {
                    while let Ok(pkt) = fwd_rx.recv().await {
                        let _ = tx.send(pkt);
                    }
                });
                println!("🚀 [HydraStream RTSP Relay] Clean stream ready at rtsp://0.0.0.0:{}/{}", port, stream_id);
            }
            Err(_) => {
                eprintln!("❌ Failed to receive SDP from source camera");
            }
        }

        if let Err(e) = server.run().await {
            eprintln!("❌ [RTSP Server Error] {}", e);
        }
    });
}
