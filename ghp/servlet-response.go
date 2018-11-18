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

func NewServletResponse(w http.ResponseWriter) *ServletResponse {
  return &ServletResponse{ w: w }
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

func (w *ServletResponse) Flush() bool {
  flusher, ok := w.w.(http.Flusher)
  if ok {
    flusher.Flush()
  }
  return ok
}
