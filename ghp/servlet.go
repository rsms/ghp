package main

import (
  "ghp"
)

type Servlet struct {
  name      string
  version   int64     // Unix nanotime of .so mtime
  serveHTTP ghp.ServeHTTP    // never nil
  stopFun   ghp.StopServlet  // may be nil
  ctx       *servletContext

  builderr  error
  srcGraph  *SrcGraph
}


// Dealloc is called when a servlet instance is no longer used and never will
// be again. Any resources can be deallocated at this point.
//
func (s *Servlet) Dealloc() {
  logf("[servlet] %q/%d dealloc", s.String(), s.version)
  s.name = ""
  s.serveHTTP = nil
  s.builderr = nil
  s.srcGraph = nil
}


func (s *Servlet) String() string {
  return s.name
}

