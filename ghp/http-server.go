package main

import (
  "fmt"
  "html"
  "io"
  "net/http"
  "os"
  "path/filepath"
  "runtime/debug"
  "strconv"
  "time"
  "path"
  "ghp"
)


var servletCache *ServletCache

type HttpServer struct {
  c *HttpServerConfig
  s *http.Server
}

func NewHttpServer(c *HttpServerConfig) *HttpServer {
  s := &HttpServer{
    c: c,
    s: &http.Server{
      Addr:           fmt.Sprintf("%s:%d", c.Address, c.Port),
      ReadTimeout:    10 * time.Second,
      WriteTimeout:   10 * time.Second,
      MaxHeaderBytes: 1 << 20,
    },
  }
  s.s.Handler = s
  // s.s.RegisterOnShutdown(s.onShutdown)
  return s
}


// func (s *HttpServer) onShutdown() {
//   // This can be used to gracefully shutdown connections that have undergone
//   // NPN/ALPN protocol upgrade or that have been hijacked.
//   // This function should start protocol-specific graceful shutdown, but
//   // should not wait for shutdown to complete.
// }


func (s *HttpServer) ListenAndServe() error {
  logf("listening on http://%s", s.s.Addr)
  return s.s.ListenAndServe()
}


func (s *HttpServer) Close() error {
  return s.s.Close()
}


const errBody404 = "<html><body><h1>404 not found</h1></body></html>\n"
const errBody500 = "<html><body><h1>500 internal server error</h1></body></html>\n"

func replyNotFound(w http.ResponseWriter) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.Header().Set("Content-Length", strconv.Itoa(len(errBody404)))
  w.WriteHeader(http.StatusNotFound)
  io.WriteString(w, errBody404)
}


func replyError(w http.ResponseWriter, message interface{}) {
  var msg, details string

  if err, ok := message.(error); ok {
    msg = err.Error()
    if e, ok := err.(*GoBuildError); ok {
      details = e.Details
    }
  } else if s, ok := message.(string); ok {
    msg = s
  }

  logf("500 internal server error: %s", msg)

  body := errBody500

  if devMode {
    body = fmt.Sprintf(
      "<html><body>" +
      "<h1>500 internal server error</h1>" +
      "<pre style='white-space:pre-wrap'>%s\n\n%s\n</pre>" +
      "</body></html>\n",
      html.EscapeString(msg),
      html.EscapeString(details),
    )
  }

  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.Header().Set("Content-Length", strconv.Itoa(len(body)))
  w.WriteHeader(http.StatusInternalServerError)
  io.WriteString(w, body)
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
func canonicalizeDirPath(w http.ResponseWriter, r *http.Request, pathname string) bool {
  cleanedPath := path.Clean(pathname)
  if cleanedPath != "/" {
    cleanedPath = cleanedPath + "/"
  }
  if pathname != cleanedPath {
    logf("redirect %q to canonical path %q", pathname, cleanedPath)
    localRedirect(w, r, cleanedPath)
    return true
  }
  return false
}


// serveServlet serves a request for a servlet.
// A servlet is always a directory with a servlet.go file.
//
func serveServlet(fspath string, d os.FileInfo, w http.ResponseWriter, r *http.Request) {
  // redirect if requested path is not canonical
  // TODO if we allow URL rerouting, this needs to be aware of it.
  if canonicalizeDirPath(w, r, r.URL.Path) {
    return
  }

  // get filename relative to pubdir
  filename, err := filepath.Rel(config.PubDir, fspath)
  if err != nil {
    replyError(w, err)
    return
  }

  s, err := servletCache.Get(filename)
  if err != nil {
    replyError(w, err)
  } else if s.serveHTTP == nil {
    replyError(w, "missing ServeHTTP")
  } else {
    req := &ghp.Request{Request: r}
    res := &ServletResponse{w: w}
    s.serveHTTP(req, res)
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
  http.ServeContent(w, r, d.Name(), d.ModTime(), f)
  // http.FileServer(http.Dir(pubdir))
}


func servePage(f *os.File, d os.FileInfo, w http.ResponseWriter, r *http.Request) {
  p, err := pageCache.Get(&buildCtx{}, f, d)
  
  if err == nil {
    err = p.Serve(w, r)
  }
  
  if err != nil {
    replyError(w, err)
  }
}


func (s *HttpServer) withFile(
  filename string,
  w http.ResponseWriter,
  fn func(f *os.File, d os.FileInfo)error,
) bool {
  f, err := os.Open(filename)
  if err != nil {
    if os.IsNotExist(err) {
      replyNotFound(w)
    } else {
      replyError(w, err)
    }
    return false
  }
  defer f.Close()

  // read file stats
  d, err := f.Stat()
  if err == nil {
    err = fn(f, d)
  }
  if err != nil {
    replyError(w, err)
    return false
  }
  return true
}


func (s *HttpServer) servePageFile(filename string, w http.ResponseWriter, r *http.Request) {
  s.withFile(filename, w, func (f *os.File, d os.FileInfo) error {
    servePage(f, d, w, r)
    return nil
  })
}


func (s *HttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  // handle panics and use as reply in development mode
  if devMode {
    defer func() {
      if r := recover(); r != nil {
        logf("panic in serve(): %v", r)
        debug.PrintStack()
        replyError(w, errorf("%v", r))
      }
    }()
  }

  // log request
  logf("%s %s (%s, %s, %s)",
    r.Method,
    r.URL.RequestURI(),
    r.Proto,
    r.Host,
    r.RemoteAddr)

  // join request path together with pubdir
  // note that URL.Path never contains ".."
  fspath := filepath.Join(config.PubDir, r.URL.Path)

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
      if name == "servlet.go" {
        // directory contains an "servlet.go" file -- treat it as a servlet
        serveServlet(fspath, d, w, r)
        return
      }
      if name == "index.html" {
        http.ServeFile(w, r, pjoin(fspath, name))
        return
      }
      if name == "index.ghp" {
        s.servePageFile(pjoin(fspath, name), w, r)
        return
      }
    }

    // directory does not contain any index file
    if config.DirList.Enabled {
      serveDirListing(file, d, w, r)
    } else {
      replyNotFound(w)
    }

  } else {
    // file
    ext := filepath.Ext(fspath)
    if ext == ".ghp" {
      servePage(file, d, w, r)
    } else {
      serveFile(file, d, w, r)
    }
  }
}
