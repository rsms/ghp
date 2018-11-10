package main

import (
  "ghp"
  "plugin"
)

type Servlet struct {
  dir       string    // servlet source package directory path
  name      string    // identifying name (e.g. "foo/bar")
  version   int64     // Unix nanotime of .so mtime
  libfile   string    // library file
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


func (s *Servlet) Load() error {
  logf("loading servlet %q from %q", s.name, s.libfile)

  o, err := plugin.Open(s.libfile)
  if err != nil {
    logf("plugin.Open failed: %v", err)
    return err
  }

  // ServeHTTP
  sym, err := o.Lookup("ServeHTTP")
  if err != nil {
    return errorf("missing ServeHTTP function")
  }
  if fn, ok := sym.(ghp.ServeHTTP); ok {
    s.serveHTTP = fn
  } else {
    return errorf("incorrect signature of ServeHTTP function")
  }

  // StopServlet (optional)
  if sym, err := o.Lookup("StopServlet"); err == nil {
    if fn, ok := sym.(ghp.StopServlet); ok {
      s.stopFun = fn
    } else {
      return errorf("incorrect signature of StopServlet function")
    }
  }

  // StartServlet (optional)
  if sym, err := o.Lookup("StartServlet"); err == nil {
    if fn, ok := sym.(ghp.StartServlet); ok {
      logf("[servlet %s] call StartServlet", s)
      fn(s.ctx)
    } else {
      return errorf("incorrect signature of StartServlet function")
    }
  }

  return nil
}


// func (s *Servlet) setVersionAndLibFile(libdir string, version int64) {
//   s.version = version
//   s.libfile = servletLibFile(libdir, s.name, version)
// }


// func servletLibFile(libdir, name string, version int64) string {
//   return pjoin(libdir, name, strconv.FormatInt(version, 10) + ".so")
// }


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

