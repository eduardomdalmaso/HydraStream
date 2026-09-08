//! Symmetric Coroutines for Zero-Runtime-Hop Stream Processing
//! ("Async Rust" - Chapter 5: Coroutines as Fundamental Computation Units)
//!
//! Preserves state and operates cooperatively across computational steps,
//! passing data directly to downstream stages without returning control to the main event loop.

use crate::codec::{NalParser, FLAG_KEYFRAME, FLAG_DELTA_FRAME};
use crate::transport::FramePayload;

/// Outcome of executing a step in a symmetric coroutine.
pub enum CoroutineYield<T> {
    /// Produces a transformed value and passes execution to the next stage.
    Continue(T),
    /// Filters out or pauses processing for this particular input item.
    Skip,
}

/// A symmetric coroutine that receives an input, mutates its internal state cooperatively,
/// and yields output directly to the next stage.
pub trait SymmetricCoroutine<Input, Output> {
    fn resume(&mut self, input: Input) -> CoroutineYield<Output>;

    /// Chains this coroutine with another symmetric coroutine without runtime polling overhead.
    fn then<Next, FinalOutput>(self, next: Next) -> ChainedCoroutine<Self, Next, Output>
    where
        Self: Sized,
        Next: SymmetricCoroutine<Output, FinalOutput>,
    {
        ChainedCoroutine {
            first: self,
            second: next,
            _marker: std::marker::PhantomData,
        }
    }
}

/// Composed symmetric coroutine executing two steps back-to-back in the same CPU stack frame.
pub struct ChainedCoroutine<First, Second, Intermediate> {
    first: First,
    second: Second,
    _marker: std::marker::PhantomData<fn(Intermediate)>,
}

impl<Input, Intermediate, Output, First, Second> SymmetricCoroutine<Input, Output>
    for ChainedCoroutine<First, Second, Intermediate>
where
    First: SymmetricCoroutine<Input, Intermediate>,
    Second: SymmetricCoroutine<Intermediate, Output>,
{
    #[inline(always)]
    fn resume(&mut self, input: Input) -> CoroutineYield<Output> {
        match self.first.resume(input) {
            CoroutineYield::Continue(inter) => self.second.resume(inter),
            CoroutineYield::Skip => CoroutineYield::Skip,
        }
    }
}

/// Coroutine that classifies video payload NAL types (H.264/H.265) and attaches metadata flags.
pub struct NalClassifierCoroutine {
    is_h265: bool,
    total_processed: u64,
}

impl NalClassifierCoroutine {
    pub fn new(is_h265: bool) -> Self {
        Self { is_h265, total_processed: 0 }
    }
}

impl SymmetricCoroutine<FramePayload, (FramePayload, u32)> for NalClassifierCoroutine {
    #[inline(always)]
    fn resume(&mut self, payload: FramePayload) -> CoroutineYield<(FramePayload, u32)> {
        self.total_processed += 1;
        let flags = if let Some(buf) = payload.host_buffer() {
            NalParser::classify_frame_flags(buf, self.is_h265)
        } else {
            FLAG_DELTA_FRAME
        };
        CoroutineYield::Continue((payload, flags))
    }
}

/// Coroutine that applies dynamic FPS governor filtering.
pub struct FpsFilterCoroutine {
    interval_us: u64,
    last_dispatched_us: u64,
    has_dispatched: bool,
}

impl FpsFilterCoroutine {
    pub fn new(target_fps: f64) -> Self {
        let interval_us = if target_fps > 0.0 {
            (1_000_000.0 / target_fps) as u64
        } else {
            0
        };
        Self {
            interval_us,
            last_dispatched_us: 0,
            has_dispatched: false,
        }
    }
}

impl SymmetricCoroutine<(FramePayload, u32), (FramePayload, u32)> for FpsFilterCoroutine {
    #[inline(always)]
    fn resume(&mut self, (payload, flags): (FramePayload, u32)) -> CoroutineYield<(FramePayload, u32)> {
        let is_keyframe = (flags & FLAG_KEYFRAME) != 0;
        let ts = payload.timestamp_us;

        // Keyframes always pass through to preserve decode stream continuity
        if is_keyframe || !self.has_dispatched || (ts.saturating_sub(self.last_dispatched_us) >= self.interval_us) {
            self.last_dispatched_us = ts;
            self.has_dispatched = true;
            CoroutineYield::Continue((payload, flags))
        } else {
            CoroutineYield::Skip
        }
    }
}
