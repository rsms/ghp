package main

import (
  "context"
  "fmt"
  "html"
  "io"
  "net"
  "net/http"
  "os"
  "path"
  "path/filepath"
  "runtime/debug"
  "strconv"
  "strings"
  "time"

  "github.com/rsms/ghp"
  "golang.org/x/crypto/acme/autocert"
)


// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
// [Lifted from go/src/net/http/server.go]
type tcpKeepAliveListener struct {
  *net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
  tc, err := ln.AcceptTCP()
  if err != nil {
    return nil, err
  }
  tc.SetKeepAlive(true)
  tc.SetKeepAlivePeriod(3 * time.Minute)
  return tc, nil
}


// --------------------------------------------------

type HttpServer struct {
  g       *Ghp
  l       net.Listener
  s       *http.Server
  c       *ServerConfig
  dirlist *HtmlDirLister
  pageIndexName string
}


func NewHttpServer(g *Ghp, c *ServerConfig) *HttpServer {
  s := &HttpServer{
    g: g,
    c: c,
  }

  if g.pageCache != nil {
    s.pageIndexName = "index" + g.pageCache.fileext
  }

  addr := c.Address
  if strings.IndexByte(addr, ':') < 0 {
    if s.c.Type == "https" {
      addr += ":443"
    } else {
      addr += ":80"
    }
  }

  s.s = &http.Server{
    Addr:           addr,
    ReadTimeout:    10 * time.Second,
    WriteTimeout:   10 * time.Second,
    MaxHeaderBytes: 1 << 20,
    ErrorLog:       logger,
    Handler:        s,
  }

  // s.s.RegisterOnShutdown(s.onShutdown)

  return s
}


func (s *HttpServer) String() string {
  return "HttpServer(" + s.c.Type + "://" + s.s.Addr + ")"
}


func (s *HttpServer) Serve() error {
  var err error

  if s.l == nil {
    return errorf("nil listener on server %v", s)
  }

  // initialize directory lister
  if s.c.DirList.Enabled {
    s.dirlist, err = NewHtmlDirLister(s.g.config.PubDir, &s.c.DirList)
    if err != nil {
      return err
    }
  } else {
    s.dirlist = nil
  }

  // wrap TCP listeners in tcpKeepAliveListener to properly configure
  // keep-alive for accepted connections.
  ln := s.l
  if tcpln, ok := ln.(*net.TCPListener); ok {
    ln = &tcpKeepAliveListener{tcpln}
  }

  if s.c.Type == "https" {
    return s.serveHttps(ln)
  } else {
    return s.serveHttp(ln)
  }
}


func (s *HttpServer) Addr() string {
  return s.s.Addr
}


func (s *HttpServer) serveHttps(l net.Listener) error {
  var certFile, keyFile string
  if s.c.Autocert != nil {
    if err := s.configureAutocert(); err != nil {
      return err
    }
  } else {
    // use certificate files
    certFile = abspath(s.c.TlsCertFile)
    keyFile = abspath(s.c.TlsKeyFile)
    if err := checkIsFile(certFile); err != nil {
      return err
    }
    if err := checkIsFile(keyFile); err != nil {
      return err
    }
  }
  logf("listening on https://%s", s.s.Addr)
  return s.s.ServeTLS(l, certFile, keyFile)
}


func (s *HttpServer) serveHttp(l net.Listener) error {
  // log warnings when TLS properties are specified but type is not https
  if s.c.TlsCertFile != "" {
    logf("warning: server config with unused tls-cert-file (not https)")
  }
  if s.c.TlsKeyFile != "" {
    logf("warning: server config with unused tls-key-file (not https)")
  }
  if s.c.Autocert != nil {
    logf("warning: server config with unused autocert config (not https)")
  }
  logf("listening on http://%s", s.s.Addr)
  return s.s.Serve(l)
}


func (s *HttpServer) Close() error {
  return s.s.Close()
}


func (s *HttpServer) Shutdown(ctx context.Context) error {
  return s.s.Shutdown(ctx)
}


// func (s *HttpServer) onShutdown() {
//   logf("HttpServer.onShutdown")
//   // This can be used to gracefully shutdown connections that have undergone
//   // NPN/ALPN protocol upgrade or that have been hijacked.
//   // This function should start protocol-specific graceful shutdown, but
//   // should not wait for shutdown to complete.
// }


func (s *HttpServer) configureAutocert() error {
  if len(s.c.Autocert.Hosts) == 0 {
    return errorf("autocert.hosts is empty in server config")
  }
  if s.c.TlsCertFile != "" {
    return errorf("both autocert and tls-cert-file in server config")
  }
  if s.c.TlsKeyFile != "" {
    return errorf("both autocert and tls-key-file in server config")
  }

  autocertCacheDir := pjoin(s.g.appCacheDir, "autocert")
  if err := os.MkdirAll(autocertCacheDir, 0700); err != nil {
    return err
  }

  m := autocert.Manager{
    Cache: autocert.DirCache(autocertCacheDir),
    Prompt: autocert.AcceptTOS,
    HostPolicy: autocert.HostWhitelist(s.c.Autocert.Hosts...),
    Email: s.c.Autocert.Email,
  }

  s.s.TLSConfig = m.TLSConfig()

  return nil
}


func (s *HttpServer) ServeHTTP(w_ http.ResponseWriter, r *http.Request) {
  w := &HttpResponse{w_}

  // handle panics and use as reply in development mode
  if devMode {
    defer func() {
      if r := recover(); r != nil {
        logf("panic in serve(): %v", r)
        debug.PrintStack()
        s.replyError(w, errorf("%v", r))
      }
    }()
  }

  // TODO: break out into
  // func (s *HttpServer) serve(w *HttpResponse, r *http.Request) error
  // and wrap in ServeHTTP to simplify replyError

  // log request
  logf("%s %s (%s, %s, %s)",
    r.Method,
    r.URL.RequestURI(),
    r.Proto,
    r.Host,
    r.RemoteAddr)

  // join request path together with pubdir
  // note that URL.Path never contains ".."
  fspath := filepath.Join(s.g.config.PubDir, r.URL.Path)

  // attempt to open requested file
  file, err := os.Open(fspath)
  if err != nil {
    // we can't read the file. Why doesn't really matter. Send 404
    s.replyNotFound(w)
    return
  }
  defer file.Close()

  // read file stats
  d, err := file.Stat()
  if err != nil {
    s.replyError(w, err)
    return
  }

  if d.IsDir() {
    // TODO: consider using filepath.Walk instead

    // directory -- look for an "index" file
    names, err := file.Readdirnames(0)
    if err != nil {
      s.replyError(w, err)
      return
    }

    for _, name := range names {
      // Note: We need to test for page before index.html as the page
      // file extension might be ".html"
      if s.g.pageCache != nil && name == s.pageIndexName {
        s.servePageFile(pjoin(fspath, name), w, r)
        return
      }
      if name == "index.html" {
        http.ServeFile(w, r, pjoin(fspath, name))
        return
      }
      if s.g.servletCache != nil && name == "servlet.go" {
        s.serveServlet(fspath, d, w, r)
        return
      }
    }

    // directory does not contain any index file
    if s.dirlist != nil {
      s.serveDirListing(file, d, w, r)
    } else {
      s.replyNotFound(w)
    }

  } else {
    // file
    ext := filepath.Ext(fspath)
    if s.g.pageCache != nil && ext == s.g.pageCache.fileext {
      s.servePage(file, d, w, r)
    } else {
      s.serveFile(file, d, w, r)
    }
  }
}


// serveServlet serves a request for a servlet.
// A servlet is always a directory with a servlet.go file.
//
func (s *HttpServer) serveServlet(fspath string, d os.FileInfo, w *HttpResponse, r *http.Request) {
  // redirect if requested path is not canonical
  // TODO if we allow URL rerouting, this needs to be aware of it.
  if s.canonicalizeDirPath(w, r, r.URL.Path) {
    return
  }

  // get filename relative to pubdir
  filename, err := filepath.Rel(s.g.config.PubDir, fspath)
  if err != nil {
    s.replyError(w, err)
    return
  }

  servlet, err := s.g.servletCache.Get(filename)
  if err != nil {
    s.replyError(w, err)
  } else if servlet.serveHTTP == nil {
    s.replyError(w, "missing ServeHTTP in servlet")
  } else {
    req := (*ghp.Request)(r)
    servlet.serveHTTP(req, w)
  }
}


func (s *HttpServer) serveDirListing(f *os.File, d os.FileInfo, w *HttpResponse, r *http.Request) {
  // redirect if requested path is not canonical
  if s.canonicalizeDirPath(w, r, r.URL.Path) {
    return
  }

  html, err := s.dirlist.RenderHtml(f.Name(), r.URL.Path)
  if err != nil {
    s.replyError(w, err)
    return
  }

  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.Header().Set("Content-Length", strconv.Itoa(len(html)))
  w.setLastModified(d.ModTime())
  w.Write(html)
}


func (s *HttpServer) serveFile(f *os.File, d os.FileInfo, w *HttpResponse, r *http.Request) {
  http.ServeContent(w, r, d.Name(), d.ModTime(), f)
}


func (s *HttpServer) servePage(f *os.File, d os.FileInfo, w *HttpResponse, r *http.Request) {
  p, err := s.g.pageCache.Get(&buildCtx{}, f, d)
  if err == nil {
    err = p.Serve(w, r)
  }
  if err != nil {
    s.replyError(w, err)
  }
}


func (s *HttpServer) servePageFile(filename string, w *HttpResponse, r *http.Request) {
  f, err := os.Open(filename)
  if err != nil {
    if os.IsNotExist(err) {
      s.replyNotFound(w)
    } else {
      s.replyError(w, err)
    }
    return
  }
  defer f.Close()

  d, err := f.Stat()
  if err != nil {
    s.replyError(w, err)
    return
  }

  s.servePage(f, d, w, r)
}




const errBody404 = "<html><body><h1>404 not found</h1></body></html>\n"
const errBody500 = "<html><body><h1>500 internal server error</h1></body></html>\n"

func (s *HttpServer) replyNotFound(w *HttpResponse) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.Header().Set("Content-Length", strconv.Itoa(len(errBody404)))
  w.WriteHeader(http.StatusNotFound)
  io.WriteString(w, errBody404)
}


func (s *HttpServer) replyError(w *HttpResponse, message interface{}) {
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
func (s *HttpServer) replyLocalRedirect(w *HttpResponse, r *http.Request, newPath string) {
  if q := r.URL.RawQuery; q != "" {
    newPath += "?" + q
  }
  w.Header().Set("Location", newPath)
  w.WriteHeader(http.StatusMovedPermanently)
}


// canonicalizeDirPath calls replyLocalRedirect and returns true if path does
// not end in a slash and/or if path is not canoncial (e.g. contains "../")
//
func (s *HttpServer) canonicalizeDirPath(w *HttpResponse, r *http.Request, pathname string) bool {
  cleanedPath := path.Clean(pathname)
  if cleanedPath != "/" {
    cleanedPath = cleanedPath + "/"
  }
  if pathname != cleanedPath {
    logf("redirect %q to canonical path %q", pathname, cleanedPath)
    s.replyLocalRedirect(w, r, cleanedPath)
    return true
  }
  return false
}
