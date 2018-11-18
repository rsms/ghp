package main

import (
  "fmt"
  "io"
  "os"
  "path"
  "path/filepath"
  "reflect"
  "strings"
  "bytes"
  "time"
)

func logf(format string, v... interface{}) {
  if logger != nil {
    logger.Printf(format, v...)
  }
}

func errorf(format string, v... interface{}) error {
  return fmt.Errorf(format, v...)
}


func fatalf(msg interface{}, arg... interface{}) {
  var format string
  if s, ok := msg.(string); ok {
    format = s
  } else if s, ok := msg.(fmt.Stringer); ok {
    format = s.String()
  } else {
    format = fmt.Sprintf("%v", msg)
  }
  fmt.Fprintf(os.Stderr, format + "\n", arg...)
  os.Exit(1)
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

func copyfile(srcname, dstname string) (int64, error) {
  src, err := os.Open(srcname)
  if err != nil {
    return 0, err
  }
  defer src.Close()

  st, err := src.Stat()
  if err != nil {
    return 0, err
  }

  dst, err := os.OpenFile(dstname, os.O_RDWR|os.O_CREATE, st.Mode())
  if err != nil {
    return 0, err
  }
  defer dst.Close()

  return io.Copy(dst, src)
}

// freadStr reads size from file handle f and returns it as a string.
func freadStr(f *os.File, size int64) (string, error) {
  var buf bytes.Buffer
  if int64(int(size)) == size {
    // buf.Grow takes an int, not an int64
    buf.Grow(int(size))
  }
  _, err := buf.ReadFrom(f)
  return buf.String(), err
}

// relfile returns name relative to dir. If name is not rooted in dir,
// filepath.Base(name) is returned instead.
//
// Useful for including file paths in public content, e.g. a HTTP response.
//
func relfile(dir, name string) string {
  s, err := filepath.Rel(dir, name)
  if err == nil && len(s) > 0 {
    return s
  }
  return filepath.Base(name)
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

func abspathList(names string) string {
  if strings.IndexByte(names, os.PathListSeparator) != -1 {
    var v []string
    pathSep := string(os.PathListSeparator)
    for _, name := range strings.Split(names, pathSep) {
      v = append(v, abspath(name))
    }
    return strings.Join(v, pathSep)
  }
  return abspath(names)
}

// checkIsFile returns an error if filename is not a file
//
func checkIsFile(filename string) error {
  d, err := os.Stat(filename)
  if err != nil {
    return err
  }
  if !d.Mode().IsRegular() {
    return errorf("%s is not a file", filename)
  }
  return nil
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


func anySlice(values interface{}) ([]interface{}, error) {
  var items []interface{}

  val := reflect.ValueOf(values)

  if val.Kind() == reflect.Slice {
    z := val.Len()
    items = make([]interface{}, z)
    for i := 0; i < z; i++ {
      items[i] = val.Index(i).Interface()
    }
  } else {
    return items, errorf("not a slice")
  }

  // TODO: support maps

  return items, nil
}


// fanApply takes a collection of values and applies fn to each value
// in a separate goroutine.
// It "fans out", waits for all fn invocations to return and "fans in" to
// return an error, or nil.
//
// Returns first error occured, if any.
// Waits for everything to complete even in the case of an error.
//
func fanApply(values interface{}, fn func(interface{})error) error {
  errch := make(chan error)

  items, err := anySlice(values)
  if err != nil {
    return err
  }

  for _, v := range items {
    go func(v interface{}) {
      defer func() {
        if r := recover(); r != nil {
          errch <- errorf("panic %v", r)
        }
      }()
      errch <- fn(v)
    }(v)
  }

  for range items {
    e := <- errch
    if e != nil && err == nil {
      err = e
    }
  }

  return err
}
