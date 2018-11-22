package main

import (
  "flag"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "os"
  "os/signal"
  "path/filepath"
  "syscall"
  "time"
)

var (
  // set at compile time
  ghpVersion  string = "0.0.0"
  ghpBuildTag string = "src"

  // set by init()
  logger *log.Logger

  // set by main()
  ghpdir  string
  devMode bool
)


func init() {
  logger = log.New(os.Stdout, "", log.LstdFlags | log.LUTC)
}


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

  // show version?
  if showVersion {
    fmt.Printf("ghp version %s (%s)\n", ghpVersion, ghpBuildTag)
    return
  }

  // in dev mode, use a short log format
  if devMode {
    logger = log.New(os.Stdout, "", log.Ltime)
  }

  // load configuration
  config, configFile, err := loadConfig(configFile)
  if err != nil {
    fatalf("%s: %v", configFile, err)
  }

  // patch config.Go.Gopath to include ghpdir/gopath
  ghpGopath := pjoin(ghpdir, "gopath")
  if len(config.Go.Gopath) > 0 {
    sep := string(filepath.ListSeparator)
    config.Go.Gopath = ghpGopath + sep + config.Go.Gopath
  } else {
    config.Go.Gopath = ghpGopath
  }

  // make sure the go tool is available when usign servlets
  if config.Servlet.Enabled {
    if err := InitGoTool(&config.Go); err != nil {
      panic(err)
    }
  }

  // Create GHP instance
  ghp, err := NewGhp(ghpdir, config)
  if err != nil {
    panic(err)
  }

  // setup SIGHUP signal handler for graceful shutdown
  sigch := make(chan os.Signal, 1)
  signal.Notify(sigch, syscall.SIGHUP)
  gotHUP := false
  go func(){
    for {
      <-sigch  // await signal
      if !gotHUP {
        // first SIGHUP starts graceful shutdown
        gotHUP = true
        go ghp.Shutdown()
      } else {
        // second SIGHUPs causes immediate close
        go func() {
          if err := ghp.Close(); err != nil {
            panic(err)
          }
        }()
        break
      }
    }
    for {
      <-sigch
    }
  }()

  // DEBUG request something from the "example" servlet after 100ms
  if devMode {
    if len(config.Servers) > 0 {
      go debugTest(config.Servers[0])
    }
  }

  // Run GHP instance
  if err := ghp.Main(); err != nil {
    panic(err)
  }
}


func debugTest(sc *ServerConfig) {
  host := fmt.Sprintf("%s://%s", sc.Type, sc.Address)

  GET := func(url string) {
    res, err := http.Get(url)
    if err != nil {
      logf("GET %s failed: %s", url, err.Error())
      return
    }
    logf("GET %s => %s (reading body...)", url, res.Status)
    defer res.Body.Close()
    body, err := ioutil.ReadAll(res.Body)
    if err != nil {
      logf("GET %s failed (reading body): %s", url, err.Error())
      return
    }
    if len(body) > 0 {
      logf("GET %s => %s\n---- body ----\n%s\n----", url, res.Status, body)
    }
  }

  time.Sleep(time.Millisecond * 200)

  // slam test servlet
  // go GET(host + "/servlet/")
  // go GET(host + "/servlet/")
  // go GET(host + "/servlet/")
  // GET(host + "/servlet/")

  GET(host + "/servlet-sleeper/")

  // GET(host + "/no-index")
  // GET(host + "/template/nopkg/wrapped.ghp")
  // GET(host + "/template/parent-chain/d.ghp")

  // slam test page
  // go GET(host + "/template/cyclic-parents/d.ghp")
  // go GET(host + "/template/cyclic-parents/d.ghp")
  // go GET(host + "/template/cyclic-parents/d.ghp")
  // GET(host + "/template/cyclic-parents/d.ghp")
}
