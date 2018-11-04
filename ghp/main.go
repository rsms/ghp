package main

import (
  "crypto/sha1"
  "encoding/base64"
  "net/http"
  "os"
  "path/filepath"
  "strings"
  "time"
  "regexp"
)

var (
  versionGit   string = "?" // set at compile time
  ghpdir       string
  config       *Config
  appBuildDir  string   // app-specific build products
  devMode      bool
)

func main() {
  // resolve public directory where all the stuff is
  var err error
  ghpdir, err = filepath.Abs(filepath.Join(os.Args[0], "..", ".."))
  if err != nil {
    panic(err)
  }

  // load configuration
  config, err = loadConfig()
  if err != nil {
    panic(err)
  }
  devMode = config.DevMode
  baseGopath := pjoin(ghpdir, "gopath")
  if len(config.Gopath) > 0 {
    config.Gopath = baseGopath + string(filepath.ListSeparator) + config.Gopath
  } else {
    config.Gopath = baseGopath
  }
  if devMode {
    logf("config:\n----"); config.writeYaml(os.Stdout); println("----");
  }

  // compute id of pubdir
  sha1sum := sha1.Sum([]byte(config.PubDir))
  pubdirId := base64.RawURLEncoding.EncodeToString(sha1sum[:])
  pubDirV := strings.Split(config.PubDir, string(filepath.Separator))
  pubDirTail := "-" + strings.Join(pubDirV[imax(0, len(pubDirV)-3):], "-")
  slugRe := regexp.MustCompile(`[^0-9A-Za-z_]+`)
  pubdirId += slugRe.ReplaceAllString(pubDirTail, "-")

  // appBuildDir
  if strings.HasPrefix(config.BuildDir, ghpdir) {
    // BuildDir is rooted in the shared ghpdir.
    // Append 
    appBuildDir = pjoin(config.BuildDir, pubdirId)
  } else {
    appBuildDir = config.BuildDir
  }

  // init servlet cache
  servletCache = NewServletCache(config.PubDir, appBuildDir, config.Gopath)
  if config.Servlet.Preload {
    err := servletCache.LoadAll()
    if err != nil {
      panic(err)
    }
  }

  if len(config.HttpServer) == 0 {
    panic("no http-server configured")
  }

  // start servers
  var servers []*HttpServer
  var errch chan error = make(chan error)
  for _, serverConfig := range config.HttpServer {
    s := NewHttpServer(serverConfig)
    servers = append(servers, s)
    go func() {
      err := s.ListenAndServe()
      select {
      case errch <- err:
      default:
      }
    }()
  }

  // DEBUG request something from the "example" servlet after 100ms
  go func() {
    time.Sleep(time.Millisecond * 100)
    // res, _ := http.Get("http://localhost:8001/example/")
    // res, _ := http.Get("http://localhost:8001/no-index")
    // res, _ := http.Get("http://localhost:8001/template/nopkg/wrapped.ghp")
    res, _ := http.Get("http://localhost:8001/template/parent-chain/d.ghp")

    // slam test
    // go http.Get("http://localhost:8001/template/cyclic-parents/d.ghp")
    // go http.Get("http://localhost:8001/template/cyclic-parents/d.ghp")
    // go http.Get("http://localhost:8001/template/cyclic-parents/d.ghp")
    // res, _ := http.Get("http://localhost:8001/template/cyclic-parents/d.ghp")

    logf("GET => %v", res)
  }()

  // wait for servers
  err = <- errch
  if err != nil {
    for _, s := range servers {
      s.Close()
    }
    panic(err)
  }
}
