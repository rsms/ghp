package main

import (
  "crypto/sha1"
  "encoding/base64"
  "fmt"
  "net/http"
  "os"
  "path/filepath"
  "regexp"
  "runtime"
  "strings"
  "time"
  "io/ioutil"
  "flag"
)

var (
  ghpVersion    string = "0.0.0" // set at compile time
  ghpVersionGit string = "?"     // set at compile time
  ghpdir        string
  config        *Config
  appBuildDir   string   // app-specific build products
  devMode       bool
)


func main() {
  // resolve public directory where all the stuff is
  var err error
  ghpdir, err = filepath.Abs(filepath.Join(os.Args[0], "..", ".."))
  if err != nil {
    panic(err)
  }

  // parse CLI flags
  var configFile string
  var showVersion bool
  flag.BoolVar(&devMode, "dev", devMode, "Run in development mode")
  flag.StringVar(&configFile, "C", configFile, "Load configuration file")
  flag.BoolVar(&showVersion, "version", showVersion, "Print version to stdout and exit")
  flag.Parse()

  // version?
  if showVersion {
    fmt.Printf("ghp version %s (%s)\n", ghpVersion, ghpVersionGit)
    return
  }

  // load configuration
  config, err = loadConfig(configFile)
  if err != nil {
    panic(err)
  }

  // patch config.Go.Gopath to include ghp's gopath
  ghpGopath := pjoin(ghpdir, "gopath")
  if len(config.Go.Gopath) > 0 {
    sep := string(filepath.ListSeparator)
    config.Go.Gopath = ghpGopath + sep + config.Go.Gopath
  } else {
    config.Go.Gopath = ghpGopath
  }

  // in dev mode, print configuration
  if devMode {
    logf("mode: development\n----")
    config.writeYaml(os.Stdout)
    println("----");
  }

  // initialize appBuildDir which is unique per go config and pubdir
  initAppBuildDir()

  // init servlet system
  if config.Servlet.Enabled {
    // make sure the go tool is available
    if err := InitGoTool(); err != nil {
      panic(err)
    }

    // If we are not recycling servlet libs, trash any old ones.
    if !config.Servlet.Recycle {
      os.RemoveAll(appBuildDir)
    }

    // setup a servlet cache
    servletCache = NewServletCache(config.PubDir, appBuildDir)
    if config.Servlet.Preload {
      err := servletCache.LoadAll()
      if err != nil {
        panic(err)
      }
    }
  }

  // must have at least one server configured
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
  go debugTest()

  // wait for servers
  err = <- errch
  if err != nil {
    for _, s := range servers {
      s.Close()
    }
    panic(err)
  }
}


func initAppBuildDir() {
  // runtimeTag used for build folder
  // Note: We assume that the go compiler and environment used to build GHP
  // is also being used to build servlets. We probably need to guarantee this
  // anyways, but this comment is here as a -CAUTION- for now.
  runtimeTag := fmt.Sprintf(
    ".%s-%s-%s-%s",
    runtime.Version(),
    runtime.Compiler,
    runtime.GOOS,
    runtime.GOARCH,
  )

  // appBuildDir
  if strings.HasPrefix(config.BuildDir, ghpdir) {
    // BuildDir is rooted in the shared ghpdir, so include pubdirId

    // compute id of pubdir
    sha1sum := sha1.Sum([]byte(config.PubDir))
    pubdirId := base64.RawURLEncoding.EncodeToString(sha1sum[:])
    pubDirV := strings.Split(config.PubDir, string(filepath.Separator))
    pubDirFragment := strings.Join(pubDirV[imax(0, len(pubDirV)-2):], "-") + "-"
    slugRe := regexp.MustCompile(`[^0-9A-Za-z_]+`)
    pubdirId = slugRe.ReplaceAllString(pubDirFragment, "-") + pubdirId

    appBuildDir = pjoin(config.BuildDir, pubdirId + runtimeTag)
  } else {
    appBuildDir = config.BuildDir + runtimeTag
  }
}


func debugTest() {
  GET := func(url string) {
    res, err := http.Get("http://localhost:8001/servlet/")
    if err != nil {
      logf("GET %s failed: %s", url, err.Error())
      return
    }
    defer res.Body.Close()
    body, err := ioutil.ReadAll(res.Body)
    if err != nil {
      logf("GET %s failed (reading body): %s", url, err.Error())
      return
    }
    if len(body) > 0 {
      logf("GET %s => %s\n----\n%s\n----", url, res.Status, body)
    } else {
      logf("GET %s => %s.", url, res.Status)
    }
  }

  time.Sleep(time.Millisecond * 100)

  // slam test servlet
  // go GET("http://localhost:8001/servlet/")
  // go GET("http://localhost:8001/servlet/")
  // go GET("http://localhost:8001/servlet/")
  GET("http://localhost:8001/servlet/")

  // GET("http://localhost:8001/no-index")
  // GET("http://localhost:8001/template/nopkg/wrapped.ghp")
  // GET("http://localhost:8001/template/parent-chain/d.ghp")

  // slam test page
  // go GET("http://localhost:8001/template/cyclic-parents/d.ghp")
  // go GET("http://localhost:8001/template/cyclic-parents/d.ghp")
  // go GET("http://localhost:8001/template/cyclic-parents/d.ghp")
  // GET("http://localhost:8001/template/cyclic-parents/d.ghp")
}
