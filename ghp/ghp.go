package main

import (
  "crypto/sha1"
  "encoding/base64"
  "fmt"
  // "log"
  // "net/http"
  "os"
  "path/filepath"
  "regexp"
  "runtime"
  "strings"
  "time"
  // "io/ioutil"
  // "flag"
)

// Ghp constitutes an entire GHP program instance.
//
type Ghp struct {
  ghpdir       string
  config       *GhpConfig
  appCacheDir  string   // app-specific data cache
  appBuildDir  string   // app-specific build products
  servers      serverSet
  servletCache *ServletCache
  pageCache    *PageCache
  zdr          *Zdr  // zero-downtime restart
  helperfuns   HelpersMap
}


func NewGhp(ghpdir string, config *GhpConfig) (*Ghp, error) {
  g := &Ghp{
    ghpdir: ghpdir,
    config: config,
  }

  // initialize appCacheDir and appBuildDir which is unique per
  // go config and pubdir
  g.initAppCacheDir()

  return g, nil
}


func (g *Ghp) Main() error {
  if devMode {
    logf("running in development mode\n----")
    println("Configuration:")
    g.config.writeYaml(os.Stdout)
    println("----")
    println("  appCacheDir:", g.appCacheDir)
    println("  appBuildDir:", g.appBuildDir)
    println("----")
  }

  // init server set
  if len(g.config.Servers) == 0 {
    return errorf("no servers configured")
  }
  for _, serverConfig := range g.config.Servers {
    s := NewHttpServer(g, serverConfig)
    g.servers.AddHttpServer(s)
  }
  AtExit(func() { g.servers.Close() }) // Make sure servers close at exit

  // init pages system
  if g.config.Pages.Enabled {
    g.helperfuns = g.buildHelpers(getBaseHelpers())
    g.pageCache = NewPageCache(g, &g.config.Pages)
  }

  // init servlet system
  if g.config.Servlet.Enabled {
    if err := g.initServlets(&g.config.Servlet); err != nil {
      return err
    }
  }

  // existing listening sockets, passed on from past process (zdr)
  var listeners []*ConnSock

  // init zero-downtime restart system (blocks on coordination)
  if g.config.Zdr.Enabled {
    var err error
    listeners, err = g.startZdr(&g.config.Zdr)
    if err != nil {
      return err
    }
    defer g.zdr.Close()
  }

  // Start listening for incoming connections
  if err := g.servers.Listen(listeners); err != nil {
    return err
  }

  // Serve. Blocks until all are done.
  if err := g.servers.Serve(); err != nil {
    return err
  }

  // Await any graceful shutdown, when zdr is enabled
  if g.zdr != nil {
    if err := g.zdr.AwaitShutdown(); err != nil {
      return err
    }
    g.zdr.Close()
  }

  return nil
}


func (g *Ghp) Close() error {
  if err := g.servers.Close(); err != nil {
    return err
  }
  if g.servletCache != nil {
    g.servletCache.Close()
  }
  return nil
}


func (g *Ghp) Shutdown() {
  if devMode {
    logf("graceful shutdown initiated")
  }

  if err := g.servers.Shutdown(); err != nil {
    logf("error shutting down servers: %v", err)
  }

  // shut down all servlets
  if g.servletCache != nil {
    g.servletCache.Shutdown()
  }

  // close zdr
  if g.zdr != nil {
    g.zdr.Close()
  }

  if devMode {
    logf("graceful shutdown completed")
  }
}


func (g *Ghp) initServlets(c *ServletConfig) error {
  builddir := pjoin(g.appBuildDir, "servlet")

  if !c.Recycle {
    os.RemoveAll(builddir)
  }

  // setup servlet cache
  g.servletCache = NewServletCache(g, c, builddir)

  if c.Preload {
    return g.servletCache.LoadAll()
  }

  return nil
}


func (g *Ghp) startZdr(c *ZdrConfig) ([]*ConnSock, error) {
  // by default, place socket file in app cache directory
  sockpath := pjoin(g.appCacheDir, "zdr.sock")

  // custom group
  if c.Group != "" {
    // base in ghp/cache dir
    sockpath = pjoin(ghpdir, "cache", "zdr." + c.Group + ".sock")
  }

  g.zdr = NewZdr(g, sockpath)

  // make sure zdr closes on program exit
  AtExit(func() {
    if g.zdr != nil {
      g.zdr.Close()
    }
  })

  // Acquire the "master" role
  timeout := 60 * time.Second
  return g.zdr.AcquireMasterRole(timeout)
}


func (g *Ghp) initAppCacheDir() {
  if strings.HasPrefix(g.config.CacheDir, ghpdir) {
    // CacheDir is rooted in the shared ghpdir, so add on pubdirId,
    // unique to each pubdir.
    sha1sum := sha1.Sum([]byte(g.config.PubDir))
    pubdirId := base64.RawURLEncoding.EncodeToString(sha1sum[:])
    pubDirV := strings.Split(g.config.PubDir, string(filepath.Separator))
    pubDirFrag := strings.Join(pubDirV[imax(0, len(pubDirV)-2):], "-") + "-"
    slugRe := regexp.MustCompile(`[^0-9A-Za-z_]+`)
    pubdirId = slugRe.ReplaceAllString(pubDirFrag, "-") + pubdirId

    g.appCacheDir = pjoin(g.config.CacheDir, pubdirId)
  }

  // runtime tag used for build folder.
  // Note: We assume that the go compiler and environment used to build GHP
  // is also being used to build servlets. We probably need to guarantee this
  // anyways, but this comment is here as a -CAUTION- for now.
  buildDirname := fmt.Sprintf(
    "build.%s-%s-%s-%s",
    runtime.Version(),
    runtime.Compiler,
    runtime.GOOS,
    runtime.GOARCH,
  )

  g.appBuildDir = pjoin(g.appCacheDir, buildDirname)
}
