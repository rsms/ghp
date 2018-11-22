package main

import (
  "context"
  "net"
  "net/http"
  "os"
)


type serverSet struct {
  httpServers []*HttpServer
}


func (ss *serverSet) AddHttpServer(s *HttpServer) {
  ss.httpServers = append(ss.httpServers, s)
}


// // TakeListeners takes ownership of listeners and applies them
// // to servers as needed
// //
// func (ss *serverSet) TakeListeners(listeners []*ConnSock) error {
// }


// Listen creates or allocates listeners for servers.
// Takes ownership of lsocks.
// Upon error, all listeners will close, so no need to call CloseListeners
// when an error occurs.
//
func (ss *serverSet) Listen(lsocks []*ConnSock) error {
  var err error

  // listeners for ss.httpServers aligned with index on ss.httpServers
  for _, s := range ss.httpServers {
    var l net.Listener

    // find lsock
    for j, ls := range lsocks {
      if ls.Proto == "tcp" && ls.Addr == s.Addr() {
        if devMode {
          logf("adopted existing listener for server %v", s)
        }
        f := os.NewFile(uintptr(ls.Fd), ls.Addr)
        if l, err = net.FileListener(f); err != nil {
          ss.CloseListeners()
          return err
        }
        // remove ls from lsocks
        lsocks = append(lsocks[:j], lsocks[j+1:]...)
        break
      }
    }

    if l == nil {
      // no lsock found; create new listener
      if l, err = net.Listen("tcp", s.Addr()); err != nil {
        ss.CloseListeners()
        return err
      }
    }

    // assign listener
    s.l = l
  }

  // close unused lsocks
  for _, ls := range lsocks {
    ls.Close()
  }

  return nil
}


// Serve runs all servers' "serve" functions in separate goroutines
// and returns once all servers are done.
// Servers must already be listening for connections.
// If an error occurs, all servers are stopped immediately.
//
func (ss *serverSet) Serve() error {
  // fan-out-in sync channel
  errch := make(chan error)

  // run each http server in a goroutine
  for _, s := range ss.httpServers {
    go ss.serveHttp(s, errch)
  }

  // wait for servers, reading their return values
  var err error
  for range ss.httpServers {
    e := <- errch
    if e != nil && err == nil {
      err = e
    }
  }
  return err
}


// runHttpServer calls s.Serve(l) and places the return result onto errch.
// Also has a recovery point for panics in s.Serve.
//
func (ss *serverSet) serveHttp(s *HttpServer, errch chan error) {
  defer func() {
    if r := recover(); r != nil {
      errch <- errorf("panic %v", r)
    }
  }()

  err := s.Serve()

  if err != nil {
    if err == http.ErrServerClosed {
      err = nil
    } else {
      // close all servers immediately so that the error propagates
      // to the caller of ListenAndServe()
      ss.Close()
    }
  }

  errch <- err
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


// CloseListeners closes all listeners on servers.
//
func (ss *serverSet) CloseListeners() error {
  var err error
  for _, s := range ss.httpServers {
    if s.l != nil {
      if e := s.l.Close(); e != nil {
        err = e
      }
      s.l = nil
    }
  }
  return err
}


func (ss *serverSet) Shutdown() error {
  return fanApply(ss.httpServers[:], func(v interface{}) error {
    // d := time.Now().Add(30 * time.Second)
    // ctx, cancel := context.WithDeadline(context.Background(), d)
    // defer cancel()
    ctx := context.Background()
    err := v.(*HttpServer).Shutdown(ctx)
    if err != nil {
      ss.Close()
    }
    return err
  })
}
