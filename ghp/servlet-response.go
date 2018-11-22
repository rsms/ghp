package main

import (
  "net/http"
)

// implements ghp.Response
//
type ServletResponse HttpResponse

func (w *ServletResponse) WriteString(s string) (int, error) {
  return w.Write([]byte(s))
}

func (w *ServletResponse) Flush() bool {
  flusher, ok := w.ResponseWriter.(http.Flusher)
  if ok {
    flusher.Flush()
  }
  return ok
}
