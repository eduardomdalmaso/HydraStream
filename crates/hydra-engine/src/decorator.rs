//! Future Decorator Pattern for Non-Intrusive Telemetry & Latency Profiling
//! ("Async Rust" - Chapter 8: The Decorator Pattern for Dynamic Feature Injection)
//!
//! Wraps any async `Future` to measure execution time, poll cycles, and errors transparently
//! without modifying the underlying business logic.

use std::future::Future;
use std::pin::Pin;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::task::{Context, Poll};
use std::time::Instant;

/// Shared telemetry sink updated by decorated Futures.
#[derive(Debug, Default)]
pub struct FutureMetricsSink {
    pub total_polls: AtomicU64,
    pub total_completions: AtomicU64,
    pub total_latency_nanos: AtomicU64,
}

impl FutureMetricsSink {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn stats(&self) -> (u64, u64, u64) {
        (
            self.total_polls.load(Ordering::Relaxed),
            self.total_completions.load(Ordering::Relaxed),
            self.total_latency_nanos.load(Ordering::Relaxed),
        )
    }
}

/// Decorator that wraps an inner `Future` and measures execution metrics.
pub struct TelemetryFuture<F> {
    inner: F,
    sink: Arc<FutureMetricsSink>,
    start_time: Option<Instant>,
    poll_count: u64,
}

impl<F> TelemetryFuture<F> {
    pub fn new(inner: F, sink: Arc<FutureMetricsSink>) -> Self {
        Self {
            inner,
            sink,
            start_time: None,
            poll_count: 0,
        }
    }
}

impl<F: Future> Future for TelemetryFuture<F> {
    type Output = F::Output;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        // Safety: We project the inner future pin safely since TelemetryFuture doesn't move inner.
        let this = unsafe { self.get_unchecked_mut() };

        if this.start_time.is_none() {
            this.start_time = Some(Instant::now());
        }

        this.poll_count += 1;
        this.sink.total_polls.fetch_add(1, Ordering::Relaxed);

        let inner_pin = unsafe { Pin::new_unchecked(&mut this.inner) };
        match inner_pin.poll(cx) {
            Poll::Ready(output) => {
                if let Some(start) = this.start_time.take() {
                    let elapsed = start.elapsed().as_nanos() as u64;
                    this.sink.total_latency_nanos.fetch_add(elapsed, Ordering::Relaxed);
                }
                this.sink.total_completions.fetch_add(1, Ordering::Relaxed);
                Poll::Ready(output)
            }
            Poll::Pending => Poll::Pending,
        }
    }
}

/// Extension trait to easily decorate any Future with telemetry.
pub trait TelemetryExt: Sized {
    fn with_telemetry(self, sink: Arc<FutureMetricsSink>) -> TelemetryFuture<Self>;
}

impl<F: Future> TelemetryExt for F {
    fn with_telemetry(self, sink: Arc<FutureMetricsSink>) -> TelemetryFuture<Self> {
        TelemetryFuture::new(self, sink)
    }
}
