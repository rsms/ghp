package main

import (
  "net/http"
  "net/url"
)

// implements ghp.Request

type RequestImpl struct {
  r *http.Request
}

func (r *RequestImpl) UnderlyingObject() interface{} {
  return r.r
}

func (r *RequestImpl) Url() *url.URL {
  return r.r.URL
}
