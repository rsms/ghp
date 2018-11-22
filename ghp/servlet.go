package main

import (
  "fmt"
  "ghp"
  "plugin"
)

type Servlet struct {
  cache     *ServletCache  // managing cache
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


func NewServlet(cache *ServletCache, dir, name string) *Servlet {
  s := &Servlet{
    cache: cache,
    dir: dir,
    name: name,
  }
  s.ctx = &servletContext{s: s}
  return s
}


func (s *Servlet) Build() error {
  logf("[servlet] building %s -> %q", s, s.libfile)

  g := NewGoTool(
    "build",
    "-buildmode=plugin",
    // "-installsuffix", "cgo",  // if env CGO_ENABLED=0
    // "-gcflags", "-p " + libfile,
    "-ldflags", "-pluginpath=" + s.libfile,  // needed for uniqueness
    "-o", s.libfile,
  )

  // set working directory to servlet's source directory
  g.Cmd.Dir = s.dir

  // run go build
  _, stderr, err := g.RunBufferedIO()
  if err != nil {
    logf("[servlet] go build failed: %s\n%s", err.Error(), stderr.String())
    return makeGoBuildError(
      fmt.Sprintf("failed to build servlet %q", s.name),
      relfile(s.cache.srcdir, s.dir),
      stderr.String(),
    )
  }

  return nil
}


func (s *Servlet) Load() error {
  logf("[servlet] loading %q from %q", s.name, s.libfile)

  o, err := plugin.Open(s.libfile)
  if err != nil {
    return errorf("plugin.Open failed: %v", err)
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


func (s *Servlet) Stop() error {
  if s.srcGraph != nil {
    s.srcGraph.Close()
    s.srcGraph = nil
  }
  if s.stopFun != nil {
    s.stopFun(s.ctx)
    s.stopFun = nil
  }
  return nil
}


func (s *Servlet) initHotReload() error {
  if s.srcGraph != nil {
    s.srcGraph.Close()
  }
  s.srcGraph = NewSrcGraph(s.dir)
  return s.srcGraph.Scan()
}

