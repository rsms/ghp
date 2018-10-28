package main

import (
  "net/http"
  "io"
)

// implements ghp.Response

type ResponseImpl struct {
  w http.ResponseWriter
}

func (w *ResponseImpl) UnderlyingObject() interface{} {
  return w.w
}

func (w *ResponseImpl) WriteString(s string) (int, error) {
  return io.WriteString(w.w, s)
}

func (w *ResponseImpl) Write(b []byte) (int, error) {
  return w.w.Write(b)
}
