//! Lock-Free Dynamic Peer Management (AtomicPtr Linked List)

use std::ptr;
use std::sync::atomic::{AtomicBool, AtomicPtr, AtomicUsize, Ordering};

pub struct PeerNode {
    pub client_id: usize,
    pub interest_mask: u32,
    pub active: AtomicBool,
    pub next: AtomicPtr<PeerNode>,
}

pub struct LockFreePeerList {
    head: AtomicPtr<PeerNode>,
    count: AtomicUsize,
}

impl LockFreePeerList {
    pub const fn new() -> Self {
        Self {
            head: AtomicPtr::new(ptr::null_mut()),
            count: AtomicUsize::new(0),
        }
    }

    /// Lock-Free Insertion via CAS loop
    pub fn register(&self, client_id: usize, interest_mask: u32) -> *mut PeerNode {
        let node = Box::into_raw(Box::new(PeerNode {
            client_id,
            interest_mask,
            active: AtomicBool::new(true),
            next: AtomicPtr::new(ptr::null_mut()),
        }));

        let mut current = self.head.load(Ordering::Relaxed);
        loop {
            unsafe { (*node).next.store(current, Ordering::Relaxed) };
            match self.head.compare_exchange_weak(
                current,
                node,
                Ordering::Release,
                Ordering::Relaxed,
            ) {
                Ok(_) => {
                    self.count.fetch_add(1, Ordering::Relaxed);
                    return node;
                }
                Err(actual) => current = actual,
            }
        }
    }

    /// Mark peer inactive without locking readers
    pub fn mark_inactive(&self, client_id: usize) {
        self.for_each(|node| {
            if node.client_id == client_id {
                node.active.store(false, Ordering::Release);
            }
        });
    }

    /// Pure Lock-Free Reader Traversal
    pub fn for_each<F: FnMut(&PeerNode)>(&self, mut callback: F) {
        let mut curr = self.head.load(Ordering::Acquire);
        while !curr.is_null() {
            unsafe {
                if (*curr).active.load(Ordering::Acquire) {
                    callback(&*curr);
                }
                curr = (*curr).next.load(Ordering::Acquire);
            }
        }
    }

    pub fn active_count(&self) -> usize {
        let mut n = 0;
        self.for_each(|_| n += 1);
        n
    }
}

impl Drop for LockFreePeerList {
    fn drop(&mut self) {
        let mut curr = self.head.swap(ptr::null_mut(), Ordering::Acquire);
        while !curr.is_null() {
            unsafe {
                let next = (*curr).next.load(Ordering::Relaxed);
                let _ = Box::from_raw(curr);
                curr = next;
            }
        }
    }
}
