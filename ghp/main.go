package main

import (
  "fmt"
  "html"
  "io"
  "log"
  "net/http"
  "os"
  "path/filepath"
  Path "path"
  "runtime/debug"
  "strconv"
  "time"
)


var (
  versionGit  string = "?" // set at compile time
  pubdir      string  // file directory root for served files
  miscdir     string  // misc files used internally by ghp
  pluginCache *PluginCache
)

const notFoundBody = "<html><body><h1>404 not found</h1></html>\n"

func replyNotFound(w http.ResponseWriter) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.Header().Set("Content-Length", strconv.Itoa(len(notFoundBody)))
  w.WriteHeader(http.StatusNotFound)
  io.WriteString(w, notFoundBody)
}

func replyError(w http.ResponseWriter, message interface{}) {
  var msg string
  if err, ok := message.(error); ok {
    msg = err.Error()
  } else if s, ok := message.(string); ok {
    msg = s
  }

  log.Printf("500 internal server error: %s", msg)
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.WriteHeader(http.StatusInternalServerError)

  // if DEBUG {
  fmt.Fprintf(w,
    "<html><body><h1>500 internal server error</h1><pre>%s\n\n%s</pre></html>\n",
    html.EscapeString(msg), string(debug.Stack()) )
  // } else {
  //   io.WriteString(w, "<html><body><h1>500 internal server error</h1></html>\n")
  // }
}

// localRedirect gives a Moved Permanently response.
// It does not convert relative paths to absolute paths like Redirect does.
//
func localRedirect(w http.ResponseWriter, r *http.Request, newPath string) {
  if q := r.URL.RawQuery; q != "" {
    newPath += "?" + q
  }
  w.Header().Set("Location", newPath)
  w.WriteHeader(http.StatusMovedPermanently)
}

var unixEpochTime = time.Unix(0, 0)

// isZeroTime reports whether t is obviously unspecified (either zero or Unix()=0).
//
func isZeroTime(t time.Time) bool {
  return t.IsZero() || t.Equal(unixEpochTime)
}

// setLastModified sets Last-Modified header if modtime != 0
//
func setLastModified(w http.ResponseWriter, modtime time.Time) {
  if !isZeroTime(modtime) {
    w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
  }
}

// canonicalizeDirPath calls localRedirect and returns true if path does not
// end in a slash and/or if path is not canoncial (e.g. contains "../")
//
func canonicalizeDirPath(w http.ResponseWriter, r *http.Request, path string) bool {
  cleanedPath := Path.Clean(path)
  if cleanedPath != "/" {
    cleanedPath = cleanedPath + "/"
  }
  if path != cleanedPath {
    log.Printf("redirect %q to canonical path %q", path, cleanedPath)
    localRedirect(w, r, cleanedPath)
    return true
  }
  return false
}


// servePlugin serves a request for a plugin
//
func servePlugin(fspath string, d os.FileInfo, w http.ResponseWriter, r *http.Request) {
  // redirect if requested path is not canonical
  // TODO if we allow URL rerouting, this needs to be aware of it.
  if canonicalizeDirPath(w, r, r.URL.Path) {
    return
  }

  // get filename relative to pubdir
  filename, err := filepath.Rel(pubdir, fspath)
  if err != nil {
    replyError(w, "unexpected condition: plugin fspath not rooted in pubdir")
    return
  }

  p, err := pluginCache.GetPlugin(filename)
  if err != nil {
    replyError(w, err)
  } else if p.ServeHTTP == nil {
    replyError(w, "missing ServeHTTP")
  } else {
    req := &RequestImpl{r: r}
    res := &ResponseImpl{w: w}
    p.ServeHTTP(req, res)
  }
}


func serveDirListing(f *os.File, d os.FileInfo, w http.ResponseWriter, r *http.Request) {
  // redirect if requested path is not canonical
  if canonicalizeDirPath(w, r, r.URL.Path) {
    return
  }

  html, err := dirlistHtml(f.Name(), r.URL.Path)
  if err != nil {
    replyError(w, err)
    return
  }

  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.Header().Set("Content-Length", strconv.Itoa(len(html)))
  setLastModified(w, d.ModTime())
  w.Write(html)
}


func serveFile(f *os.File, d os.FileInfo, w http.ResponseWriter, r *http.Request) {
  // sizeFunc := func() (int64, error) { return d.Size(), nil }
  // serveContent(w, r, d.Name(), d.ModTime(), sizeFunc, f)
  http.ServeContent(w, r, d.Name(), d.ModTime(), f)
}


func serve(w http.ResponseWriter, r *http.Request) {
  // log request
  log.Printf("%s %s (%s, %s, %s)",
    r.Method,
    r.URL.RequestURI(),
    r.Proto,
    r.Host,
    r.RemoteAddr)

  // join request path together with pubdir
  // note that URL.Path never contains ".."
  fspath := filepath.Join(pubdir, r.URL.Path)

  // attempt to open requested file
  file, err := os.Open(fspath)
  if err != nil {
    // we can't read the file. Why doesn't really matter. Send 404
    replyNotFound(w)
    return
  }
  defer file.Close()

  // read file stats
  d, err := file.Stat()
  if err != nil {
    replyError(w, err)
    return
  }

  if d.IsDir() {

    // TODO: use filepath.Walk instead
    // filepath.Walk(root, func(path string, d os.FileInfo, err error) error {}

    // directory -- look for an "index" file
    names, err := file.Readdirnames(0)
    if err != nil {
      replyError(w, err)
      return
    }

    for _, name := range names {
      if name == "index.go" {
        // directory contains an "index.go" file -- treat it as a plugin
        servePlugin(fspath, d, w, r)
        return
      }
      if name == "index.html" {
        http.ServeFile(w, r, filepath.Join(pubdir, r.URL.Path))
        return
      }
    }

    // directory does not contain any index file -- reply with file list
    serveDirListing(file, d, w, r)

  } else {
    // file -- send the file
    serveFile(file, d, w, r)
  }
}


func main() {
  // resolve public directory where all the stuff is
  ghpdir, err := filepath.Abs(filepath.Join(os.Args[0], "..", ".."))
  if err != nil {
    panic(err)
  }
  pubdir = filepath.Join(ghpdir, "pub")
  miscdir = filepath.Join(ghpdir, "misc")
  builddir := filepath.Join(ghpdir, "build", "plugins")
  gopath := filepath.Join(ghpdir, "gopath")

  // init plugin cache
  pluginCache = NewPluginCache(pubdir, builddir, gopath)
  pluginCache.Options.WatchFS = true

  // register handler for all the things
  http.HandleFunc("/", serve)

  // DEBUG request something from the "example" plugin after 100ms
  go func() {
    time.Sleep(time.Millisecond * 100)
    res, _ := http.Get("http://localhost:8002/example/")
    // res, _ := http.Get("http://localhost:8002/no-index")
    log.Println(res)
  }()

  print("listening on http://localhost:8002/\n")
  log.Fatal(http.ListenAndServe("127.0.0.1:8002", nil))
}

// Notes:
//
// - The plugin package does not provide any way of unloading plugins.
//   In fact, it retains an internal map of all loaded plugins.
//
// - There's no dlclose() equivalent in the plugin package
//
// - Idea: <id>=sha1(plugin-source) store "plugins/<name>/<id>.so" and
//   have the loader sha1(plugin-source) and load the file at the expected
//   path. Keep track of past versions that have been loaded and when the
//   count hits a certain watermark, restart the service to release memory.
//   Optionally try implementing dlclose() and call it on old plugins after
//   a new version has been loaded.
//
