"""
HydraStream Python SDK
Zero-overhead video frame multiplexing & POSIX SHM consumer for computer vision.
"""

from .reader import connect, StreamConsumer

__version__ = "0.1.0"
__all__ = ["connect", "StreamConsumer"]
