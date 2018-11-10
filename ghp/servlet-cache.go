package main

import (
  "os"
  "path/filepath"
  "sort"
  "sync"
  "strconv"
  "time"
)

type ServletCache struct {
  srcdir   string  // where servlet sources are located
  builddir string  // where servlet .so files are stored

  items    map[string]*Servlet  // ready servlets
  itemsmu  sync.RWMutex

  buildq   map[string]chan *Servlet
  buildqmu sync.Mutex
}


func NewServletCache(srcdir, builddir string) *ServletCache {
  return &ServletCache{
    srcdir:   srcdir,
    builddir: builddir,
    items:    make(map[string]*Servlet),
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
          servletdir, err := filepath.Rel(config.PubDir, dir)
          if err != nil {
            maybeSendError(errch, err)
          } else if _, err := c.Get(servletdir); err != nil {
            maybeSendError(errch, err)
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
func (c *ServletCache) Get(name string) (*Servlet, error) {
  s := c.GetCached(name)
  if s != nil && (s.srcGraph == nil || s.version > s.srcGraph.mtime) {
    return s, s.builderr
  }
  return c.Build(name, s)
}


// GetCached unconditionally returns a servlet if one is found in cache.
// Caller should check p.builderr
//
func (c *ServletCache) GetCached(name string) *Servlet {
  if devMode {
    assert(len(name) > 0, "empty name")
    assert(!filepath.IsAbs(name), "absolute name %v", name)
  }
  c.itemsmu.RLock()
  s := c.items[name]
  c.itemsmu.RUnlock()
  return s
}


// Build builds the servlet from source file f.
//
// If prevs is not nil, its resources may be transferred to the newly-build
// servlet. For instance, its source graph.
//
// This is concurrency-safe; multiple calls while a page is being built are
// all multiplexed to the same "build".
//
func (c *ServletCache) Build(name string, prevs *Servlet) (*Servlet, error) {
  c.buildqmu.Lock()

  if c.buildq == nil {
    c.buildq = make(map[string]chan *Servlet)
  } else if bch, ok := c.buildq[name]; ok {
    // already in progress of being built

    // done with buildq
    c.buildqmu.Unlock()

    // Wait for other goroutine who started the build
    s := <- bch
    
    return s, s.builderr
  }

  // If we get here, name was not found in buildq

  // Calling goroutine is responsible for building. Setup condition in build.
  bch := make(chan *Servlet)
  c.buildq[name] = bch
  c.buildqmu.Unlock()  // done with buildq

  // Create new servlet
  s := NewServlet(c.servletDir(name), name)

  // Build
  if prevs == nil {  // no previous servlet instance
    if config.Servlet.HotReload {
      err := s.initHotReload()
      if err != nil {
        logf("[servlet %q] initHotReload error: %s", s, err.Error())
      }
    }
    c.buildAndLoadServletInit(s)
  } else {
    c.buildAndLoadServlet(s)
  }

  // Place result in items map (full write-lock)
  c.itemsmu.Lock()
  if prevs != nil {
    // transfer source graph from prevs to s
    s.srcGraph, prevs.srcGraph = prevs.srcGraph, s.srcGraph
  }
  c.items[name] = s  // Note: replaces prevs, if any
  c.itemsmu.Unlock()

  // Cleanup any replaced servlet
  if prevs != nil {
    go func() {
      os.Remove(prevs.libfile)
      if prevs.stopFun != nil {
        prevs.stopFun(prevs.ctx)
      }
      prevs.Dealloc()
    }()
  }

  // Clear buildq and send on chan
  c.buildqmu.Lock()

  // Send build page to anyone who is listening
  broadcast_loop:
  for {
    select {
    case bch <- s:
    default:
      break broadcast_loop
    }
  }
  
  delete(c.buildq, name)
  c.buildqmu.Unlock()

  return s, s.builderr
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


// func (c *ServletCache) rmServletLibFile(servletName string, version int64) {
// }


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
  libOK := false
  s.libfile = c.findServletLibFile(s.name)
  s.version = time.Now().UnixNano()

  // check if we have a valid library file
  if len(s.libfile) > 0 {
    libstat, err := os.Stat(s.libfile)
    libOK = err == nil
    if libOK {
      if s.srcGraph != nil && libstat.ModTime().UnixNano() < s.srcGraph.mtime {
        // source code is newer than library file
        s.version = s.srcGraph.mtime
        libOK = false
      } else {
        // set s.version from libfile
        s.version = parseLibFileVersion(s.libfile)
      }
    }
  }

  // build if needed
  for {
    if !libOK {
      s.libfile = c.servletLibFile(s.name, s.version)
      if err := s.Build(); err != nil {
        s.builderr = err
        return
      }
    }

    // load servlet
    if err := s.Load(); err != nil {
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
  s.version = time.Now().UnixNano()
  s.libfile = c.servletLibFile(s.name, s.version)

  if err := s.Build(); err != nil {
    s.builderr = err
    return
  }

  if err := s.Load(); err != nil {
    s.builderr = err
  }
}



