package main

import (
  "fmt"
  "net/http"
  "time"
)

type HttpResponse struct {
  http.ResponseWriter
}

// setLastModified sets Last-Modified header if modtime != 0
//
func (w *HttpResponse) setLastModified(modtime time.Time) {
  if !isZeroTime(modtime) {
    w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
  }
}

func (w *HttpResponse) WriteString(s string) (int, error) {
  return w.Write([]byte(s))
}

func (w *HttpResponse) Print(a interface{}) (int, error) {
  return fmt.Fprint(w, a)
}

func (w *HttpResponse) Printf(format string, arg... interface{}) (int, error) {
  return fmt.Fprintf(w, format, arg...)
}

func (w *HttpResponse) Flush() bool {
  flusher, ok := w.ResponseWriter.(http.Flusher)
  if ok {
    flusher.Flush()
  }
  return ok
}
