package main

import (
  "bytes"
  "fmt"
  "ghp"
  "log"
  "os"
  "os/exec"
  "path/filepath"
  "plugin"
  "sort"
  "strconv"
  "strings"
  "sync"
  "time"
)

type PluginServeFun = func(ghp.Request, ghp.Response)

type Plugin struct {
  Name      string
  Version   int64     // Unix nanotime of .so mtime
  ServeHTTP PluginServeFun

  builderr  error
  srcGraph  *SrcGraph
}


// Dealloc is called when a plugin instance is no longer used and never will
// be again. Any resources can be deallocated at this point.
//
func (p *Plugin) Dealloc() {
  log.Printf("[P] %q/%d dealloc", p.Name, p.Version)
  p.Name = ""
  p.ServeHTTP = nil
  p.builderr = nil
  p.srcGraph = nil
}


type PluginCacheOptions struct {
  WatchFS bool  // watch file system for changes
}


type PluginCache struct {
  Options   PluginCacheOptions

  srcdir    string  // where plugin sources are located
  builddir  string  // where plugin .so files are stored
  gopath    string  // contains ghp; added to GOPATH

  plugins   map[string]*Plugin  // ready plugins
  pluginsmu sync.RWMutex

  buildq    map[string]*sync.Cond  // plugins in process of building
  buildqmu  sync.Mutex
}


func NewPluginCache(srcdir, builddir, gopath string) *PluginCache {
  return &PluginCache{
    srcdir:   srcdir,
    builddir: builddir,
    gopath:   gopath,
    plugins:  make(map[string]*Plugin),
  }
}


// GetPlugin returns a plugin for name.
// name should be a relative filename rooted in c.srcdir.
//
// Always returns a valid Plugin, even on errors. This means that repeat
// requests for a fauly returns the same error and plugin, until either
// the plugin has be rebuilt or emoved.
//
func (c *PluginCache) GetPlugin(name string) (*Plugin, error) {
  // fetch already-built plugin
  c.pluginsmu.RLock()
  p := c.plugins[name]
  c.pluginsmu.RUnlock()
  if p != nil {
    // We found a plugin
    if p.srcGraph != nil {
      // Source code is being observed.
      // Check if library file is newer than source code
      if p.Version > p.srcGraph.mtime {
        // up to date
        log.Printf("[PC] use preloaded %q/%d (src/%d)",
          name, p.Version, p.srcGraph.mtime)
        return p, p.builderr
      }
      // Outdated -- continue with building the plugin.
      log.Printf("[PC] outdated %q/%d", name, p.Version)
    } else {
      // Not observing source code. We're good.
      log.Printf("[PC] use preloaded %q/%d", name, p.Version)
      return p, p.builderr
    }
  }

  // Build and load plugin, or wait until it's been loaded
  c.buildqmu.Lock()

  if c.buildq == nil {
    c.buildq = make(map[string]*sync.Cond)
  } else if bc := c.buildq[name]; bc != nil {
    // Plugin is already in progress of being built
    c.buildqmu.Unlock()

    // Wait for other goroutine who started the build
    bc.Wait()

    // Read plugin from plugins map
    c.pluginsmu.RLock()
    p = c.plugins[name]
    c.pluginsmu.RUnlock()
    if p == nil {
      panic("c.plugins[name]==nil")
    }
    return p, p.builderr
  }
  
  // Calling goroutine is responsible for building the plugin.
  // Setup a condition in the buildq
  bc := sync.NewCond(&sync.Mutex{})
  c.buildq[name] = bc
  c.buildqmu.Unlock()

  // Build & load
  p2 := &Plugin{Name: name}
  if p == nil {
    // watch source for changes, if enabled
    if c.Options.WatchFS {
      srcdir := c.pluginDir(p2.Name)
      p2.srcGraph = NewSrcGraph(srcdir)
      if err := p2.srcGraph.Scan(); err != nil {
        p2.builderr = err
        log.Printf("[PC] error while scanning %q: %s", srcdir, err.Error())
      }
    }
    p2.Version = time.Now().UnixNano()
    c.buildAndLoadPluginInit(p2)
  } else {
    p2.Version = time.Now().UnixNano()
    c.buildAndLoadPlugin(p2)
  }

  // Place result in plugins map (full write-lock)
  c.pluginsmu.Lock()
  if p != nil {
    // transfer source graph from p to p2
    p2.srcGraph, p.srcGraph = p.srcGraph, p2.srcGraph
  }
  c.plugins[name] = p2
  c.pluginsmu.Unlock()

  // Wake all/any other goroutines that are waiting for the build to complete.
  // Bracket this with locking of the buildq, and clearing out the cond.
  c.buildqmu.Lock()
  bc.Broadcast()
  delete(c.buildq, name)
  c.buildqmu.Unlock()

  // Deallocate any replaced plugin
  if p != nil {
    os.Remove(c.pluginLibFile(p.Name, p.Version))
    p.Dealloc()
  }

  return p2, p2.builderr
}


func (c *PluginCache) pluginLibFile(pluginName string, version int64) string {
  return pjoin(c.builddir, pluginName, strconv.FormatInt(version, 10) + ".so")
}


func (c *PluginCache) findPluginLibFile(pluginName string) string {
  libdir := pjoin(c.builddir, pluginName)
  
  file, err := os.Open(libdir)
  if err != nil {
    return ""
  }
  defer file.Close()

  names, err := file.Readdirnames(10)
  if err != nil {
    log.Printf("[PC] unable to read directory %q: %s", libdir, err.Error())
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


func (c *PluginCache) rmPluginLibFile(pluginName string, version int64) {
}


func (c *PluginCache) pluginDir(pluginName string) string {
  return pjoin(c.srcdir, pluginName)
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


func (c *PluginCache) buildAndLoadPluginInit(p *Plugin) {
  libfile := c.findPluginLibFile(p.Name)
  log.Printf("findPluginLibFile(%q) => %q", p.Name, libfile)

  libOK := false

  // check if we have a valid library file
  if len(libfile) > 0 {
    libstat, err := os.Stat(libfile)
    libOK = err == nil
    if libOK {
      if p.srcGraph != nil && libstat.ModTime().UnixNano() < p.srcGraph.mtime {
        // source code is newer than library file
        log.Printf("[PC] source is newer than %q", libfile)
        p.Version = p.srcGraph.mtime
        libOK = false
      } else {
        // set p.Version from libfile
        p.Version = parseLibFileVersion(libfile)
        log.Printf("[PC] parseLibFileVersion(%q) => %d", libfile, p.Version)
      }
    } else {
      log.Printf("err: %s", err.Error())
    }
  }

  // build if needed
  if !libOK {
    libfile = c.pluginLibFile(p.Name, p.Version)
    if err := c.buildPlugin(p, libfile); err != nil {
      p.builderr = err
      return
    }
  }

  // load plugin
  if err := c.loadPlugin(p, libfile); err != nil {
    p.builderr = err
  }
}


func (c *PluginCache) buildAndLoadPlugin(p *Plugin) {
  libfile := c.pluginLibFile(p.Name, p.Version)

  if err := c.buildPlugin(p, libfile); err != nil {
    p.builderr = err
    return
  }

  if err := c.loadPlugin(p, libfile); err != nil {
    p.builderr = err
  }
}


func (c *PluginCache) buildPlugin(p *Plugin, libfile string) error {
  srcdir := c.pluginDir(p.Name)
  // libfile := c.pluginLibFile(p.Name, p.Version)
  // builddir := pjoin(c.builddir, p.Name)

  log.Printf("building plugin %q -> %q", p.Name, libfile)

  // TODO: make-like individual-files build cache, where we only
  // rebuild the .go files that doesn't have corresponding up-to-date
  // .o files in a cache directory.

  // TODO: consider resolving "go" at startup and panic if it can't be found.

  // build plugin as package
  cmd := exec.Command(
    "go", "build",
    "-buildmode=plugin",
    // "-gcflags", "-p " + libfile,
    "-ldflags", "-pluginpath=" + libfile,  // needed for uniqueness
    "-o", libfile,
  )

  // set working directory to plugin's source directory
  cmd.Dir = srcdir

  // configure env
  // TODO: only do this once (can use sync.Once)
  gopathPrefix := "GOPATH="
  for _, ent := range os.Environ() {
    if strings.HasPrefix(ent, gopathPrefix) {
      GOPATH := ent[len(gopathPrefix):]
      newPrefix := gopathPrefix + c.gopath + ":"
      if !strings.HasPrefix(ent, newPrefix) {
        ent = newPrefix + GOPATH
      }
    }
    cmd.Env = append(cmd.Env, ent)
  }

  // buffer output
  var outbuf bytes.Buffer
  var errbuf bytes.Buffer
  cmd.Stdout = &outbuf
  cmd.Stderr = &errbuf

  // execute program; errors if exit status != 0
  if err := cmd.Run(); err != nil {
    log.Printf("build failure: %s (%q)\n", err.Error(), errbuf.String())
    return fmt.Errorf("failed to build plugin %q", p.Name)
  }

  // XXX DEBUG
  if outbuf.Len() > 0 {
    fmt.Printf("stdout: %q\n", outbuf.String())
  }
  if errbuf.Len() > 0 {
    fmt.Printf("stderr: %q\n", errbuf.String())
  }

  // set p.Version to lib file mtime
  libstat, err := os.Stat(libfile)
  if err != nil {
    return err
  }
  p.Version = libstat.ModTime().UnixNano()

  return nil
}


func (c *PluginCache) loadPlugin(p *Plugin, libfile string) error {
  log.Printf("loading plugin %q from %q", p.Name, libfile)

  o, err := plugin.Open(libfile)
  if err != nil {
    return err
  }

  sym, err := o.Lookup("ServeHTTP")
  if err != nil {
    return fmt.Errorf("missing ServeHTTP function")
  }

  if ServeHTTP, ok := sym.(PluginServeFun); ok {
    p.ServeHTTP = ServeHTTP
  } else {
    return fmt.Errorf("incorrect signature of ServeHTTP function")
  }

  return nil
}



/*

if dir_mtime > cache_mtime
  some file was added or removed from the directory, possibly source files

if stat(*.go) > cache_mtime
  source files changed

- remember all relative imported and *.go files on compilation
- when validating cache:
  - if dir_mtime > cache_mtime, look for new or missing *.go files
  - check memorized source files' mtime vs cache_mtime

*/
