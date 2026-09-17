"""
HydraStream Python Zero-Copy Shared Memory Writer
Writes raw BGR/RGB frames into a POSIX lock-free shared memory ring buffer.
"""

import os
import mmap
import struct
import time
from typing import Optional
import numpy as np

HYDRA_MAGIC = 0x48594452  # "HYDR"
HYDRA_VERSION = 1
DEFAULT_SLOTS = 16
SHM_HEADER_SIZE = 64
SLOT_HEADER_SIZE = 64


class StreamWriter:
    """Zero-Copy POSIX Shared Memory Ring Buffer Writer."""

    def __init__(
        self,
        stream_id: str,
        width: int,
        height: int,
        format_id: int = 2,  # 2: BGR24, 1: RGB24
        slot_count: int = DEFAULT_SLOTS,
    ):
        self.stream_id = stream_id
        self.width = width
        self.height = height
        self.format_id = format_id
        self.slot_count = slot_count

        bytes_per_pixel = 4 if format_id == 4 else 3
        self.frame_bytes_len = width * height * bytes_per_pixel
        self.slot_size = SLOT_HEADER_SIZE + self.frame_bytes_len
        self.total_size = SHM_HEADER_SIZE + (self.slot_count * self.slot_size)

        shm_dir = "/dev/shm" if os.path.exists("/dev/shm") else os.environ.get("TMPDIR", "/tmp")
        self.shm_path = os.path.join(shm_dir, f"hydra_{stream_id}")

        self._file = open(self.shm_path, "wb+")
        self._file.truncate(self.total_size)
        self._mmap = mmap.mmap(self._file.fileno(), self.total_size, access=mmap.ACCESS_WRITE)

        # Write 64-byte aligned ShmHeader
        hdr = struct.pack(
            "<IIIIIIIIQ24s",
            HYDRA_MAGIC,
            HYDRA_VERSION,
            self.width,
            self.height,
            self.format_id,
            self.slot_count,
            self.slot_size,
            0,  # notify_seq
            0,  # write_sequence
            b"\x00" * 24,
        )
        self._mmap[:SHM_HEADER_SIZE] = hdr
        self._mmap.flush()

        self.seq = 0

    def write_frame(self, frame: np.ndarray, timestamp_us: Optional[int] = None) -> int:
        """Writes a numpy frame (BGR/RGB) into the next ring buffer slot."""
        if timestamp_us is None:
            timestamp_us = int(time.time() * 1_000_000)

        raw_data = frame.tobytes() if isinstance(frame, np.ndarray) else frame
        payload_size = len(raw_data)

        self.seq += 1
        slot_idx = self.seq % self.slot_count
        slot_offset = SHM_HEADER_SIZE + (slot_idx * self.slot_size)

        slot_hdr = struct.pack(
            "<QQQII32s",
            self.seq,
            timestamp_us,
            self.seq,
            payload_size,
            0,
            b"\x00" * 32,
        )
        self._mmap[slot_offset : slot_offset + SLOT_HEADER_SIZE] = slot_hdr
        self._mmap[slot_offset + SLOT_HEADER_SIZE : slot_offset + SLOT_HEADER_SIZE + payload_size] = raw_data

        # Update write_sequence atomically at offset 32
        struct.pack_into("<Q", self._mmap, 32, self.seq)
        return self.seq

    def close(self):
        if self._mmap:
            self._mmap.close()
        if self._file:
            self._file.close()
        if os.path.exists(self.shm_path):
            try:
                os.remove(self.shm_path)
            except Exception:
                pass


def create_writer(
    stream_id: str,
    width: int,
    height: int,
    format_id: int = 2,
    slot_count: int = DEFAULT_SLOTS,
) -> StreamWriter:
    """Creates a new active HydraStream shared memory ring buffer writer."""
    return StreamWriter(stream_id, width, height, format_id, slot_count)
