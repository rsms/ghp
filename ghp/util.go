package main

import (
  "fmt"
  "os"
  Path "path"
)

// assert calls panic() if cond is not true.
// Any values provided with info is included in the panic message
//
func assert(cond bool, info... interface{}) {
  if !cond {
    msg := "assertion failure"
    for _, v := range info {
      msg += fmt.Sprintf(" %+v", v)
    }
    panic(msg)
  }
}

func pathIsDotRelative(path string) bool {
  if path[0] == '.' && len(path) > 1 {
    c1 := path[1]
    return c1 == os.PathSeparator || (len(path) > 2 && c1 == '.' && path[2] == os.PathSeparator)
  }
  return false
}

// path join
func pjoin(s ...string) string {
  return Path.Join(s...)
}
