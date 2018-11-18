package ghp

import (
  // "net/url"
  "net/http"
)

// StartServlet is called when a servlet is initialized.
//
type StartServlet = func(ServletContext)

// StopServlet is called just after the servlet was hot-reloaded;
// replaced by a newer instance.
//
// It is guaranteed that no other calls to this instance of the servlet
// will occur at or after the point in time when this function is called.
//
// The servlet should stop any running tasks and deallocate any persistent
// resources.
//
type StopServlet = func(ServletContext)

// ServeHTTP is called to serve a HTTP request.
// It's the servlet's full and lone responsibility to handle the request.
// This is essentially a go net/http.Handler function, so anything you'd do
// in a net/http.Handler function, you can do here.
//
type ServeHTTP = func(*Request, Response)

// ServletContext represents the servlet instance itself.
//
type ServletContext interface {
  Name() string     // servlet name
  Version() string  // instance version
}

// Request represents a HTTP request.
//
type Request struct {
  *http.Request
}

// Response represents a HTTP response.
// Implements io.Writable
// Implements http.ResponseWriter
//
type Response interface {
  Header() http.Header
  Write([]byte) (int, error)
  WriteString(string) (int, error)
  WriteHeader(statusCode int)
  Flush() bool  // returns true on success
}
