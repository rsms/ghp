package main

import (
  "os"
  "os/signal"
  "sync"
  "syscall"
)

var (
  atExitLock sync.Mutex
  atExitFuns []func()
)

func AtExit(fn func()) {
  atExitLock.Lock()
  defer atExitLock.Unlock()
  if len(atExitFuns) == 0 {
    sigch := make(chan os.Signal, 2)
    signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
    go atExitHandler(sigch)
  }
  atExitFuns = append(atExitFuns, fn)
}

// Exit runs any registered exit functions in the inverse order they were
// registered and then exits with the specified status.
func atExitHandler(ch chan os.Signal) {
  <-ch  // await signal

  runfn := func(fn func()) {
    defer func() {
      if err := recover(); err != nil {
        logf("error in AtExit function: %+v\n", err)
      }
    }()
    fn()
  }

  atExitLock.Lock()
  for i := len(atExitFuns) - 1; i >= 0; i-- {
    runfn(atExitFuns[i])
  }
  os.Exit(1)
}
