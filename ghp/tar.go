package main

import (
  "archive/tar"
  "errors"
  "io"
  "os"
)


type TarEntType byte
const (
  TarEntOther = TarEntType(0)
  TarEntFile = TarEntType('f')
  TarEntDir = TarEntType('/')
  TarEntSymlink = TarEntType('@')
)


// TarStop signals "stop iteration"
var TarStop = errors.New("stop")


// TarVisitor is called for a specific archive entry with information about
// that entry along with a reader that can read the contents of the entry.
type TarVisitor = func (t TarEntType, name string, r io.Reader) error


// TarIterate calls the visitor function on each entry of the archive,
// read from inputr.
// The visitor can return TarStop to stop iteration early.
//
func TarIterate(inputr io.Reader, visitor TarVisitor) error {
  r := tar.NewReader(inputr)

  for {
    header, err := r.Next()

    if err != nil {
      if err == io.EOF {
        break
      }
      return err
    }

    var t TarEntType
    switch header.Typeflag {
    case tar.TypeReg, tar.TypeLink:
      t = TarEntFile
    case tar.TypeDir:
      t = TarEntDir
    case tar.TypeSymlink:
      t = TarEntSymlink
    }

    if err = visitor(t, header.Name, r); err != nil {
      if err == TarStop {
        break
      }
      return err
    }
  }

  return nil
}


// TarExtractFile is a convenience function for extracting one specific file
// from a TAR archive. matchname is the filename to look for in the archive
// e.g. "foo/bar" and dstname is the local filename where to write the file.
//
func TarExtractFile(r io.Reader, matchname, dstname string) error {
  var found bool

  err := TarIterate(r, func (t TarEntType, name string, r io.Reader) error {
    if t == TarEntFile && name == matchname {
      found = true

      // destination file
      f, err := os.OpenFile(dstname, os.O_WRONLY|os.O_CREATE, 0755)
      if err != nil {
        return err
      }
      defer f.Close()

      // copy from archive to file
      _, err = io.Copy(f, r)
      if err != nil {
        return err
      }

      return TarStop
    }
    return nil
  })

  if err == nil && !found {
    err = errorf("file not found in tar archive %q", matchname)
  }

  return err
}
