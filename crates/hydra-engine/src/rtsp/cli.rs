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
