package main

import (
  "net/http"
  "time"
  "context"
)


type serverSet struct {
  httpServers []*HttpServer
}


func (ss *serverSet) AddHttpServer(s *HttpServer) {
  ss.httpServers = append(ss.httpServers, s)
}


func (ss *serverSet) ListenAndServe() error {
  return fanApply(ss.httpServers[:], func(v interface{}) error {
    err := v.(*HttpServer).ListenAndServe()
    if err != nil {
      if err == http.ErrServerClosed {
        err = nil
      } else {
        // close all servers immediately so that the error propagates
        // to the caller of ListenAndServe()
        ss.Close()
      }
    }
    return err
  })
}


// Close immediately closes all servers
//
func (ss *serverSet) Close() error {
  var err error
  for _, s := range ss.httpServers {
    if e := s.Close(); e != nil {
      err = e
    }
  }
  return err
}


func (ss *serverSet) Shutdown() error {
  return fanApply(ss.httpServers[:], func(v interface{}) error {
    d := time.Now().Add(30 * time.Second)
    ctx, cancel := context.WithDeadline(context.Background(), d)
    defer cancel()
    err := v.(*HttpServer).Shutdown(ctx)
    if err != nil {
      ss.Close()
    }
    return err
  })
}
