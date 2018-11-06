package main

import (
  "ghp"
)

type Servlet struct {
  dir       string    // servlet source package directory path
  name      string    // identifying name (e.g. "foo/bar")
  version   int64     // Unix nanotime of .so mtime
  ctx       *servletContext
  serveHTTP ghp.ServeHTTP    // never nil
  stopFun   ghp.StopServlet  // may be nil
  builderr  error
  srcGraph  *SrcGraph        // may be nil
}


func NewServlet(dir, name string) *Servlet {
  s := &Servlet{
    dir: dir,
    name: name,
  }
  s.ctx = &servletContext{s: s}
  return s
}


// Dealloc is called when a servlet instance is no longer used and never will
// be again. Any resources can be deallocated at this point.
//
func (s *Servlet) Dealloc() {
  logf("[servlet] %q/%d dealloc", s.String(), s.version)
  s.name = ""
  s.serveHTTP = nil
  s.builderr = nil
  if s.srcGraph != nil {
    s.srcGraph.Close()
    s.srcGraph = nil
  }
}


func (s *Servlet) String() string {
  return s.name
}


func (s *Servlet) initHotReload() error {
  if s.srcGraph != nil {
    s.srcGraph.Close()
  }
  s.srcGraph = NewSrcGraph(s.dir)
  return s.srcGraph.Scan()
}

