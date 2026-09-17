"""
HydraStream Python Zero-Copy Shared Memory SDK
Directly maps POSIX shared memory ring buffer into NumPy ndarray.
"""

import os
import mmap
import struct
import time
from typing import Optional, Tuple, Generator
import numpy as np

HYDRA_MAGIC = 0x48594452  # "HYDR"
SHM_HEADER_SIZE = 64
SLOT_HEADER_SIZE = 64


class StreamConsumer:
    """Zero-Copy POSIX Shared Memory Consumer for HydraStream."""

    def __init__(self, stream_id: str):
        self.stream_id = stream_id
        self.shm_path = f"/dev/shm/hydra_{stream_id}"
        if not os.path.exists(self.shm_path):
            tmp_path = os.path.join(os.environ.get("TMPDIR", "/tmp"), f"hydra_{stream_id}")
            if os.path.exists(tmp_path):
                self.shm_path = tmp_path
            else:
                raise FileNotFoundError(f"HydraStream SHM buffer not found for '{stream_id}' at {self.shm_path}")

        self._file = open(self.shm_path, "rb")
        self._mmap = mmap.mmap(self._file.fileno(), 0, access=mmap.ACCESS_READ)

        header_bytes = self._mmap[:32]
        (
            magic,
            self.version,
            self.width,
            self.height,
            self.format_id,
            self.slot_count,
            self.slot_size,
            self.notify_seq,
        ) = struct.unpack("<IIIIIIII", header_bytes)

        if magic != HYDRA_MAGIC:
            raise ValueError(f"Invalid HydraStream signature: {hex(magic)}")

        self.last_seq = 0

    def read_latest(self) -> Optional[Tuple[int, bytes]]:
        """Reads the latest available raw frame bytes without copying unneeded slots."""
        (write_seq,) = struct.unpack_from("<Q", self._mmap, offset=32)
        if write_seq == 0 or write_seq <= self.last_seq:
            return None

        slot_idx = write_seq % self.slot_count
        slot_offset = SHM_HEADER_SIZE + (slot_idx * self.slot_size)

        slot_hdr_bytes = self._mmap[slot_offset : slot_offset + 32]
        (seq, timestamp_us, _, payload_size, _) = struct.unpack("<QQQII", slot_hdr_bytes)

        data_start = slot_offset + SLOT_HEADER_SIZE
        data_end = data_start + payload_size
        frame_bytes = self._mmap[data_start:data_end]

        self.last_seq = seq
        return timestamp_us, frame_bytes

    def read_latest_ndarray(self) -> Optional[Tuple[int, np.ndarray]]:
        """Reads latest frame directly as a numpy ndarray without decode copying."""
        res = self.read_latest()
        if res is None:
            return None
        ts, frame_bytes = res
        arr = np.frombuffer(frame_bytes, dtype=np.uint8)
        if self.format_id in (1, 2):  # RGB24 or BGR24
            return ts, arr.reshape((self.height, self.width, 3))
        return ts, arr

    def stream(self, target_fps: float = 30.0) -> Generator[bytes, None, None]:
        """Generator that yields frames at the throttled target FPS."""
        min_interval = 1.0 / max(target_fps, 0.1)
        last_yield = 0.0

        while True:
            now = time.time()
            if now - last_yield >= min_interval:
                res = self.read_latest()
                if res is not None:
                    _, frame = res
                    last_yield = now
                    yield frame
            time.sleep(0.001)

    def close(self):
        if self._mmap:
            self._mmap.close()
        if self._file:
            self._file.close()


def connect(stream_id: str) -> StreamConsumer:
    """Connects to an active HydraStream shared memory ring buffer."""
    return StreamConsumer(stream_id)
