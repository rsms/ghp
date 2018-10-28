package main

import (
  "sync/atomic"
)

// atomicStoreMaxInt64 stores v to addr if v is larger than the current
// value at addr.
// Returns true if value at addr was updated.
//
func atomicStoreMaxInt64(addr *int64, v int64) bool {
  // optimistic simple load here since the CAS will fail if we had a different
  // value in our core's L cache, and we'll retry.
  curr := *addr
  if v > curr {
    if atomic.CompareAndSwapInt64(addr, curr, v) {
      return true
    }
    // failed -- load with memory barrier in case another thread has a more
    // recent value
    curr = atomic.LoadInt64(addr)
    if v > curr {
      return atomic.CompareAndSwapInt64(addr, curr, v)
    }
  }
  return false
}
