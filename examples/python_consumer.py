#!/usr/bin/env python3
"""
Example: Consuming HydraStream POSIX SHM in Python without cv2.VideoCapture.
"""

import sys
import os
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '..', 'sdk', 'python')))

import hydrastream

def main():
    stream_id = "cam_entrance_01"
    print(f"🐍 Connecting to HydraStream zero-copy SHM for '{stream_id}'...")

    try:
        consumer = hydrastream.connect(stream_id)
        print(f"✅ Connected! Resolution: {consumer.width}x{consumer.height}, Slots: {consumer.slot_count}")
        
        frame_count = 0
        for frame_bytes in consumer.stream(target_fps=5.0):
            frame_count += 1
            print(f"  [ANALYTICS] Processed frame #{frame_count} ({len(frame_bytes)} bytes) at 5.0 FPS")
            if frame_count >= 10:
                break
                
        consumer.close()
        print("🎉 Successfully demonstrated Zero-Copy frame delivery!")
    except FileNotFoundError:
        print(f"ℹ️ Stream '{stream_id}' not currently active in /dev/shm. Start the engine to stream live.")

if __name__ == "__main__":
    main()
