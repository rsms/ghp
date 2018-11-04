package main

import (
  "bytes"
  "ghp"
  "os"
  "os/exec"
  "path/filepath"
  "plugin"
  "sort"
  "sync"
  "strconv"
  "time"
)

type ServletCache struct {
  srcdir      string  // where servlet sources are located
  builddir    string  // where servlet .so files are stored
  gopath      string  // contains ghp; added to GOPATH

  servlets   map[string]*Servlet  // ready servlets
  servletsmu sync.RWMutex

  buildq     map[string]*sync.Cond  // servlets in process of building
  buildqmu   sync.Mutex
}


func NewServletCache(srcdir, builddir, gopath string) *ServletCache {
  return &ServletCache{
    srcdir:      srcdir,
    builddir:    builddir,
    gopath:      gopath,
    servlets:    make(map[string]*Servlet),
  }
}


func (c *ServletCache) LoadAll() error {
  var wg sync.WaitGroup
  errch := make(chan error, 10)

  // scan pubdir for servlets
  err := FileScan(config.PubDir, func (dir string, names []string) error {
    // look at directory entries
    for _, name := range names {
      if name == "servlet.go" {
        // it's a servlet
        wg.Add(1)
        go func() {
          servletdir, err := relPubPath(dir)
          if err != nil {
            maybeSendError(errch, err)
          } else {
            _, err := c.GetServlet(servletdir)
            if err != nil {
              maybeSendError(errch, err)
            }
          }
          wg.Done()
        }()
        return filepath.SkipDir  // do no visit subdirectories
      }
    }
    return nil
  })
  if err != nil {
    return nil
  }

  // wait for servlets to finish loading
  logf("waiting for servlets to finish loading")
  wg.Wait()

  // read results
  result_loop:
  for {
    select {
    case err := <- errch:
      if err != nil {
        return err
      }
    default:
      break result_loop
    }
  }

  return nil
}


// GetServlet returns a servlet for name.
// name should be a relative filename rooted in pubdir.
//
// Always returns a valid Servlet, even on errors. This means that repeat
// requests for a fauly returns the same error and servlet, until either
// the servlet has be rebuilt or emoved.
//
func (c *ServletCache) GetServlet(name string) (*Servlet, error) {
  if devMode {
    if len(name) == 0 {
      return nil, errorf("empty name")
    }
    if filepath.IsAbs(name) {
      return nil, errorf("absolute name %v provided to GetServlet", name)
    }
  }

  // fetch already-built servlet
  c.servletsmu.RLock()
  s := c.servlets[name]
  c.servletsmu.RUnlock()

  if s != nil {
    // We found a servlet
    if s.srcGraph != nil {
      // Source code is being observed.
      // Check if library file is newer than source code
      if s.version > s.srcGraph.mtime {
        // up to date
        // logf("[servlet] use preloaded %q/%d (src/%d)",
        //   name, s.version, s.srcGraph.mtime)
        return s, s.builderr
      }
      // Outdated -- continue with building the servlet.
      // logf("[servlet] outdated %q/%d", name, s.version)
    } else {
      // Not observing source code. We're good.
      // logf("[servlet] use preloaded %q/%d", name, s.version)
      return s, s.builderr
    }
  }

  // Build and load servlet, or wait until it's been loaded
  c.buildqmu.Lock()

  if c.buildq == nil {
    c.buildq = make(map[string]*sync.Cond)
  } else if bc := c.buildq[name]; bc != nil {
    // Servlet is already in progress of being built
    c.buildqmu.Unlock()

    // Wait for other goroutine who started the build
    bc.Wait()

    // Read servlet from servlets map
    c.servletsmu.RLock()
    s = c.servlets[name]
    c.servletsmu.RUnlock()
    if s == nil {
      panic("c.servlets[name]==nil")
    }
    return s, s.builderr
  }
  
  // Calling goroutine is responsible for building the servlet.
  // Setup a condition in the buildq
  bc := sync.NewCond(&sync.Mutex{})
  c.buildq[name] = bc
  c.buildqmu.Unlock()

  // Build & load
  s2 := &Servlet{name: name}
  s2.ctx = &servletContext{s: s2}
  if s == nil {
    // watch source for changes, if enabled
    if config.Servlet.HotReload {
      srcdir := c.servletDir(s2.name)
      s2.srcGraph = NewSrcGraph(srcdir)
      if err := s2.srcGraph.Scan(); err != nil {
        s2.builderr = err
        logf("[servlet] error while scanning %q: %s", srcdir, err.Error())
      }
    }
    s2.version = time.Now().UnixNano()
    c.buildAndLoadServletInit(s2)
  } else {
    s2.version = time.Now().UnixNano()
    c.buildAndLoadServlet(s2)
  }

  // Place result in servlets map (full write-lock)
  c.servletsmu.Lock()
  if s != nil {
    // transfer source graph from s to s2
    s2.srcGraph, s.srcGraph = s.srcGraph, s2.srcGraph
  }
  c.servlets[name] = s2
  c.servletsmu.Unlock()

  // Wake all/any other goroutines that are waiting for the build to complete.
  // Bracket this with locking of the buildq, and clearing out the cond.
  c.buildqmu.Lock()
  bc.Broadcast()
  delete(c.buildq, name)
  c.buildqmu.Unlock()

  // Deallocate any replaced servlet
  if s != nil {
    os.Remove(c.servletLibFile(s.name, s.version))
    if s.stopFun != nil {
      s.stopFun(s.ctx)
    }
    s.Dealloc()
  }

  return s2, s2.builderr
}


func (c *ServletCache) servletLibFile(servletName string, version int64) string {
  return pjoin(c.builddir, servletName, strconv.FormatInt(version, 10) + ".so")
}


func (c *ServletCache) findServletLibFile(servletName string) string {
  libdir := pjoin(c.builddir, servletName)
  
  file, err := os.Open(libdir)
  if err != nil {
    return ""
  }
  defer file.Close()

  names, err := file.Readdirnames(10)
  if err != nil {
    logf("[servlet] unable to read directory %q: %s", libdir, err.Error())
    return ""
  }

  sort.Strings(names)

  for i := len(names) - 1; i >= 0; i-- {
    if filepath.Ext(names[i]) == ".so" {
      return pjoin(libdir, names[i])
    }
  }

  return ""
}


func (c *ServletCache) rmServletLibFile(servletName string, version int64) {
}


func (c *ServletCache) servletDir(servletName string) string {
  return pjoin(c.srcdir, servletName)
}


func parseLibFileVersion(libfile string) int64 {
  s := filepath.Base(libfile)
  s = s[:len(s)-3]  // excl ".so"
  v, err := strconv.ParseInt(s, 10, 64)
  if err != nil {
    panic("invalid lib filename " + libfile + ": " + err.Error())
  }
  return v
}


func (c *ServletCache) buildAndLoadServletInit(s *Servlet) {
  libfile := c.findServletLibFile(s.name)
  libOK := false

  // check if we have a valid library file
  if len(libfile) > 0 {
    libstat, err := os.Stat(libfile)
    libOK = err == nil
    if libOK {
      if s.srcGraph != nil && libstat.ModTime().UnixNano() < s.srcGraph.mtime {
        // source code is newer than library file
        s.version = s.srcGraph.mtime
        libOK = false
      } else {
        // set s.version from libfile
        s.version = parseLibFileVersion(libfile)
      }
    }
  }

  // build if needed
  for {
    if !libOK {
      libfile = c.servletLibFile(s.name, s.version)
      if err := c.buildServlet(s, libfile); err != nil {
        s.builderr = err
        return
      }
    }

    // load servlet
    if err := c.loadServlet(s, libfile); err != nil {
      if libOK {
        // We tried loading an existing servlet, but it failed.
        // This could happen if the servlet was built with a now-outdated
        // library, or different version of go.
        logf("[servlet] failed to load preexisting %s: %s", s, err.Error())
        // Continue loop and cause rebuild.
        s.version = time.Now().UnixNano() // update version
        libOK = false
        continue
      } else {
        s.builderr = err
      }
    }

    return
  }
}


func (c *ServletCache) buildAndLoadServlet(s *Servlet) {
  libfile := c.servletLibFile(s.name, s.version)

  if err := c.buildServlet(s, libfile); err != nil {
    s.builderr = err
    return
  }

  if err := c.loadServlet(s, libfile); err != nil {
    s.builderr = err
  }
}


func (c *ServletCache) buildServlet(s *Servlet, libfile string) error {
  srcdir := c.servletDir(s.name)
  // libfile := c.servletLibFile(s.name, s.version)
  // builddir := pjoin(c.builddir, s.name)

  logf("[servlet] building %s -> %q", s, libfile)

  // TODO: make-like individual-files build cache, where we only
  // rebuild the .go files that doesn't have corresponding up-to-date
  // .o files in a cache directory.

  // TODO: consider resolving "go" at startup and panic if it can't be found.

  // build servlet as plugin package
  cmd := exec.Command(
    "go", "build",
    "-buildmode=plugin",
    // "-gcflags", "-p " + libfile,
    "-ldflags", "-pluginpath=" + libfile,  // needed for uniqueness
    "-o", libfile,
  )

  // set working directory to servlet's source directory
  cmd.Dir = srcdir

  // set env
  cmd.Env = config.getGoProcEnv()

  // buffer output
  var outbuf bytes.Buffer
  var errbuf bytes.Buffer
  cmd.Stdout = &outbuf
  cmd.Stderr = &errbuf

  // execute program; errors if exit status != 0
  if err := cmd.Run(); err != nil {
    logf("[servlet] build failure: %s (%q)\n", err.Error(), errbuf.String())
    return errorf("failed to build servlet %q", s.name)
  }

  // if outbuf.Len() > 0 {
  //   logf("stdout: %q\n", outbuf.String())
  // }
  // if errbuf.Len() > 0 {
  //   logf("stderr: %q\n", errbuf.String())
  // }

  // set s.version to lib file mtime
  libstat, err := os.Stat(libfile)
  if err != nil {
    return err
  }
  s.version = libstat.ModTime().UnixNano()

  return nil
}


func (c *ServletCache) loadServlet(s *Servlet, libfile string) error {
  logf("loading servlet %q from %q", s.name, libfile)

  o, err := plugin.Open(libfile)
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