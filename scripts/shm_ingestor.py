#!/usr/bin/env python3
"""
HydraStream High-Speed SHM Buffer Ingestor
Reads local relay RTSP and writes zero-copy frames to /dev/shm/hydra_<stream_id>
"""

import sys
import os
import time
import cv2
import signal

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "sdk", "python"))
import hydrastream.writer as writer

stream_id = sys.argv[1] if len(sys.argv) > 1 else "platform1"
rtsp_url = sys.argv[2] if len(sys.argv) > 2 else f"rtsp://127.0.0.1:8554/{stream_id}"

running = True

def handle_signal(sig, frame):
    global running
    running = False

signal.signal(signal.SIGINT, handle_signal)
signal.signal(signal.SIGTERM, handle_signal)

os.environ["OPENCV_FFMPEG_CAPTURE_OPTIONS"] = "rtsp_transport;tcp|fflags;nobuffer|flags;low_delay"

print(f"🚀 [HydraStream SHM Ingestor] Starting for stream: {stream_id} <- {rtsp_url}")

shm_writer = None

while running:
    cap = cv2.VideoCapture(rtsp_url)
    cap.set(cv2.CAP_PROP_BUFFERSIZE, 1)

    if not cap.isOpened():
        print(f"⚠️ [HydraStream SHM Ingestor] Waiting for stream {rtsp_url}...")
        time.sleep(1.0)
        continue

    print(f"✅ [HydraStream SHM Ingestor] Connected to {rtsp_url}. Writing to /dev/shm/hydra_{stream_id}")

    try:
        while running:
            ret, frame = cap.read()
            if not ret or frame is None:
                print("⚠️ [HydraStream SHM Ingestor] Stream interrupted, reconnecting...")
                break

            h, w = frame.shape[:2]
            if shm_writer is None or shm_writer.width != w or shm_writer.height != h:
                if shm_writer:
                    shm_writer.close()
                shm_writer = writer.create_writer(stream_id, width=w, height=h, format_id=2, slot_count=16)

            shm_writer.write_frame(frame)
    finally:
        cap.release()

if shm_writer:
    shm_writer.close()

print("🛑 [HydraStream SHM Ingestor] Stopped.")
