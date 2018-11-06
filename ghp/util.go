package main

import (
  "fmt"
  "log"
  "os"
  "path"
  "path/filepath"
  "time"
)

func logf(format string, v... interface{}) {
  log.Printf(format, v...)
}

func errorf(format string, v... interface{}) error {
  return fmt.Errorf(format, v...)
}

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

// pubfilename returns a publicly-presentable filename.
//
// If it's relative to PubDir, then "/PubDir/filename" is returned,
// otherwise the basename of the filename is returned.
//
// Useful for including filenames in messages e.g. sent as a HTTP response.
//
func pubfilename(filename string) string {
  s, err := filepath.Rel(config.PubDir, filename)
  if err == nil && len(s) > 0 {
    return "/" + s
  }
  return filepath.Base(filename)
}

func pathIsDotRelative(name string) bool {
  if name[0] == '.' && len(name) > 1 {
    c1 := name[1]
    return c1 == os.PathSeparator || (len(name) > 2 && c1 == '.' && name[2] == os.PathSeparator)
  }
  return false
}

// path join
func pjoin(s ...string) string {
  return path.Join(s...)
}

// abspath returns an absolute representation of path.
// If the path is not absolute it will be joined with the current working
// directory.
func abspath(name string) string {
  name2, err := filepath.Abs(name)
  if err != nil {
    panic(err)
  }
  return name2
}

// countByte returns the number of occurances of b in s
func countByte(s string, b byte) int {
  n, i, z := 0, 0, len(s)
  for i < z {
    if s[i] == b {
      n++
    }
    i++
  }
  return n
}

// trySendError attempts to put err on channel errch
func maybeSendError(errch chan error, err error) bool {
  select {
  case errch <- err:
    return true
  default:
    return false
  }
}

func imax(a, b int) int {
  if a > b {
    return a
  }
  return b
}


var unixEpochTime = time.Unix(0, 0)

// isZeroTime reports whether t is obviously unspecified (either zero or Unix()=0).
//
func isZeroTime(t time.Time) bool {
  return t.IsZero() || t.Equal(unixEpochTime)
}
