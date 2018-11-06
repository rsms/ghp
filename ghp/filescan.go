package main

import (
  "os"
  "path/filepath"
  "sync"
)

type FileScanVisitor = func (dir string, names []string) error


func FileScan(dir string, visitor FileScanVisitor) error {
  f, err := os.Open(dir)
  if err != nil {
    return err
  }
  defer f.Close()

  names, err := f.Readdirnames(-1)
  if err != nil {
    return err
  }

  if err := visitor(dir, names); err != nil {
    if err == filepath.SkipDir {
      err = nil
    }
    return err
  }

  // visit subdirectories
  var wg    sync.WaitGroup
  var errch chan error

  for _, name := range names {
    filename := pjoin(dir, name)
    d, err := os.Lstat(filename)
    if err != nil {
      if os.IsNotExist(err) {
        // File disappeared between readdir + stat.
        // Just treat it as if it didn't exist.
        continue
      }
    }

    if !d.IsDir() {
      continue
    }

    if errch == nil {
      errch = make(chan error, 1)
    }

    wg.Add(1)
    go func() {
      if err := FileScan(filename, visitor); err != nil {
        maybeSendError(errch, err)
      }
      wg.Done()
    }()
  }

  // errch is non-nil if we started scans of subdirectories
  if errch != nil {

    // wait for subdirectory scans to complete
    wg.Wait()

    // grab first error that occured, if any
    select {
      case err := <- errch:
        return err
      default: // no error
    }
  }

  return nil
}
