//! Read-Copy-Update (RCU) Pattern for Dynamic Configuration

use std::ptr;
use std::sync::Arc;
use std::sync::atomic::{AtomicPtr, Ordering};

pub struct RcuConfig<T> {
    ptr: AtomicPtr<Arc<T>>,
}

impl<T> RcuConfig<T> {
    pub fn new(initial: T) -> Self {
        let arc = Arc::new(initial);
        let raw = Box::into_raw(Box::new(arc));
        Self {
            ptr: AtomicPtr::new(raw),
        }
    }

    /// Lock-Free Instantaneous Read
    pub fn load(&self) -> Arc<T> {
        unsafe {
            let boxed_ptr = self.ptr.load(Ordering::Acquire);
            (*boxed_ptr).clone()
        }
    }

    /// Atomic Update (Read-Copy-Update) with Release ordering
    pub fn update(&self, new_val: T) {
        let new_arc = Arc::new(new_val);
        let new_raw = Box::into_raw(Box::new(new_arc));
        let old_raw = self.ptr.swap(new_raw, Ordering::AcqRel);

        unsafe {
            let _ = Box::from_raw(old_raw);
        }
    }
}

impl<T> Drop for RcuConfig<T> {
    fn drop(&mut self) {
        let raw = self.ptr.swap(ptr::null_mut(), Ordering::Acquire);
        if !raw.is_null() {
            unsafe {
                let _ = Box::from_raw(raw);
            }
        }
    }
}
