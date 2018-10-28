package ghp

import (
  "net/url"
)

type Request interface {
  Url() *url.URL
  UnderlyingObject() interface{}  // *http.Request
}

type Response interface {
  WriteString(string) (int, error)
  Write([]byte) (int, error)
  UnderlyingObject() interface{}  // http.ResponseWriter
}
