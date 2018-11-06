package main

import (
  "net/http"
  "io"
)

// implements ghp.Response
//
type ServletResponse struct {
  w http.ResponseWriter
}

func (w *ServletResponse) Header() http.Header {
  return w.w.Header()
}

func (w *ServletResponse) WriteString(s string) (int, error) {
  return io.WriteString(w.w, s)
}

func (w *ServletResponse) Write(b []byte) (int, error) {
  return w.w.Write(b)
}

func (w *ServletResponse) WriteHeader(statusCode int) {
  w.w.WriteHeader(statusCode)
}
