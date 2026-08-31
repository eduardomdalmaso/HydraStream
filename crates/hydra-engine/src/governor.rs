//! Smart FPS Governor for Intelligent Per-Consumer Frame Sub-Sampling

pub struct FpsGovernor {
    target_fps: f64,
    min_interval_us: u64,
    last_dispatched_us: Option<u64>,
    frames_passed: u64,
    frames_dropped: u64,
}

impl FpsGovernor {
    pub fn new(target_fps: f64) -> Self {
        let target_fps = if target_fps <= 0.0 { 30.0 } else { target_fps };
        let min_interval_us = (1_000_000.0 / target_fps) as u64;

        Self {
            target_fps,
            min_interval_us,
            last_dispatched_us: None,
            frames_passed: 0,
            frames_dropped: 0,
        }
    }

    pub fn set_target_fps(&mut self, target_fps: f64) {
        self.target_fps = if target_fps <= 0.0 { 30.0 } else { target_fps };
        self.min_interval_us = (1_000_000.0 / self.target_fps) as u64;
    }

    /// Evaluates if the incoming frame should be dispatched.
    /// Returns true if dispatched, false if dropped.
    pub fn should_dispatch(&mut self, timestamp_us: u64) -> bool {
        match self.last_dispatched_us {
            None => {
                self.last_dispatched_us = Some(timestamp_us);
                self.frames_passed += 1;
                true
            }
            Some(last) => {
                if timestamp_us >= last + self.min_interval_us {
                    self.last_dispatched_us = Some(timestamp_us);
                    self.frames_passed += 1;
                    true
                } else {
                    self.frames_dropped += 1;
                    false
                }
            }
        }
    }

    pub fn stats(&self) -> (u64, u64) {
        (self.frames_passed, self.frames_dropped)
    }

    pub fn target_fps(&self) -> f64 {
        self.target_fps
    }
}
