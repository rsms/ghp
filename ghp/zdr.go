package main

import (
  "io"
  "net"
  "os"
  "path/filepath"
  "strings"
  "syscall"
  "time"
)

const (
  cmdTakeOver = "take-over"
  cmdFdInfo = "fd-info"
)

// Zero-downtime restart
type Zdr struct {
  g          *Ghp
  sockpath   string
  masterln   net.Listener
  masterlnfd int
  shutdownch chan error
}


func NewZdr(g *Ghp, sockpath string) *Zdr {
  return &Zdr{
    g: g,
    sockpath: sockpath,
    masterlnfd: -1,
  }
}


// AwaitShutdown blocks until shutdown has completed.
// If no shutdown was requested, returns immediately.
// Error returns is the first error that occured during shutdown, if any.
//
func (z *Zdr) AwaitShutdown() error {
  if z.shutdownch == nil {
    return nil
  }
  return <- z.shutdownch
}


// AcquireMasterRole will attempt to acquire the role as "master", taking
// it over from any other program currently running with a zdr active on
// z.sockpath.
//
// This function returns after the previous master program has successfully
// given up the role as master, and this program has acquired "listen" rights
// on z.sockpath.
//
// If timeout is <=0 then this function waits forever.
//
func (z *Zdr) AcquireMasterRole(timeout time.Duration) ([]*ConnSock, error) {
  // deadline for acquisition
  deadline := time.Now().Add(timeout)

  // setup shutdown channel for whenever we shut down
  z.shutdownch = make(chan error, 1)

  // build unix socket address
  addr := &net.UnixAddr{ Net: "unix", Name: z.sockpath }

  // Assume no other process is running and attempt to become master
  retry_becomeInitialMaster:
  err := z.becomeInitialMaster(addr)
  if !isAddrInUse(err) {
    // success or system error
    return nil, err
  }

  // If we get here, another process is currently master.

  // Ask to take over the role.
  conns, err := z.takeOverMaster(addr, deadline)
  if isConnRefused(err) {
    // possible race condition where the master quit in between us attempting
    // listen and dialing. Might also be the case of a crashed master, in which
    // case we need to remove the socket file.
    os.Remove(z.sockpath)
    goto retry_becomeInitialMaster
  }
  return conns, err
}


// becomeInitialMaster attempts to become the listener of the shared socket.
// It will fail if another process is currently listening.
// On success, the calling process is the master.
func (z *Zdr) becomeInitialMaster(addr *net.UnixAddr) error {
  retried := true

  listen_:
  ln, err := net.ListenUnix("unix", addr)

  if err != nil {
    if !retried {
      if os.IsNotExist(err) || strings.Contains(err.Error(), "directory") {
        os.MkdirAll(filepath.Dir(z.sockpath), 0744)
        retried = true
        goto listen_
      }
    }
    return err
  }

  z.masterln = ln

  // acquired master role!
  // dispatch master loop (reads and writes messages)
  go z.masterLoop()

  return nil
}


func (z *Zdr) takeOverMaster(addr *net.UnixAddr, deadline time.Time) ([]*ConnSock, error) {
  // connect to the current master
  conn, err := net.DialUnix("unix", nil, addr)
  if err != nil {
    return nil, err
  }

  // connected
  defer conn.Close()
  conn.SetReadDeadline(deadline)

  mw := NewIpcMsgWriter(conn)

  // send request to take over the role as master
  if err := mw.Write(cmdTakeOver); err != nil {
    return nil, err
  }

  // receive fd info from master
  mr := NewIpcMsgReader(conn)
  fdinfo, err := mr.Read(cmdFdInfo)
  if err != nil {
    return nil, err
  }
  nfds := len(fdinfo.Args)

  // receive master listener fd
  fds, err := FdExchangeRecvFDs(conn, nfds)
  if err != nil {
    return nil, errorf("FdExchangeRecvFDs: %v", err)
  }
  if len(fds) < nfds {
    return nil, errorf("too few fds received from master")
  }

  // construct masterln with the first received FD
  z.masterlnfd = fds[0]
  f := os.NewFile(uintptr(z.masterlnfd), z.sockpath)
  z.masterln, err = net.FileListener(f)
  if err != nil {
    return nil, err
  }

  // make list of ConnectedSock from rest of FDs
  var conns []*ConnSock
  for i := 1; i < nfds; i++ {
    s, err := ParseConnSock(fdinfo.Args[i])
    if err != nil {
      return nil, err
    }
    s.Fd = fds[i]
    conns = append(conns, s)
  }

  // acquired master role!
  // dispatch master loop (reads and writes messages)
  go z.masterLoop()

  return conns, nil
}


// conn is the connection to the requestor
//
func (z *Zdr) releaseMasterRole(conn net.Conn) error {
  // Send all of our listener FDs, starting with the zdr socket

  // fds
  fds := make([]int, 1 + len(z.g.servers.httpServers))
  fduris := make([]string, len(fds))

  // access zdr listener fd
  lnfd, err := getListenerFd(z.masterln)
  if err != nil {
    return err
  }
  fds[0] = lnfd
  fduris[0] = "unix:" + z.sockpath

  for i, s := range z.g.servers.httpServers {
    // get FD of server listener
    fd, err := getListenerFd(s.l)
    if err != nil {
      return err
    }

    // add to fds arrat
    fds[i + 1] = fd
    fduris[i + 1] = "tcp:" + s.Addr()

    // detach listener from server
    s.l = nil
  }

  // send fduris
  mw := NewIpcMsgWriter(conn)
  if err := mw.Write(cmdFdInfo, fduris...); err != nil {
    return err
  }

  // send fds
  err = FdExchangeSendFDs(conn, fds...)
  if err != nil {
    return err
  }

  // detach master listener
  z.masterln = nil
  z.masterlnfd = -1

  // shutdown
  if err := z.shutdown(); err != nil {
    return err
  }

  // Close listener to signal OK
  return nil
}


// shutdown calls z.g.Shutdown in a safe manner and sends the result
// on z.shutdownch
func (z *Zdr) shutdown() error {
  var err error

  // Call z.g.Shutdown which will block until shutdown has completed
  func() {
    defer func() {
      if r := recover(); r != nil {
        err = errorf("panic in Zdr g.Shutdown: %v", r)
      }
    }()
    z.g.Shutdown()
  }()

  z.shutdownch <- err

  return nil
}


func (z *Zdr) Close() {
  if z.masterln != nil {
    z.masterln.Close()
    z.masterln = nil
  }
  if z.shutdownch != nil {
    close(z.shutdownch)
    z.shutdownch = nil
  }
}


func (z *Zdr) masterLoop() {
  var err error
  var conn net.Conn

  for {
    if conn != nil {
      conn.Close()
      conn = nil
    }

    // Handle each connection in a serial manner to avoid race conditions.
    conn, err = z.masterln.Accept()
    if err != nil {
      if !isUseOfClosedConn(err) {
        logf("[zdr] error in Accept: %v", err)
      }
      // else: z.masterln closed -- end masterLoop
      return
    }

    // read a message, expecting "take over" request
    mr := NewIpcMsgReader(conn)
    if _, err := mr.Read(cmdTakeOver); err != nil {
      conn.Close()
      if err == io.EOF {
        logf("[zdr] %p disconnected", conn)
      } else {
        logf("[zdr] %p error %v (closing connection)", conn, err)
      }

      // accept another request
      continue
    }

    // The requestor wants to take over the master role. Give it up.
    if err := z.releaseMasterRole(conn); err != nil {
      logf("[zdr] failed to release master role: %v", err)

      // accept another request
      continue
    }

    // success -- end masterLoop since z.masterln is now invalid.
    conn.Close()
    return
  }
}


// getListenerFd returns the file descriptor for a net.Listener
//
func getListenerFd(l net.Listener) (int, error) {
  var err error
  var rconn syscall.RawConn

  // access rconn
  if ln, ok := l.(*net.TCPListener); ok {
    rconn, err = ln.SyscallConn()
  } else if ln, ok := l.(*net.UnixListener); ok {
    rconn, err = ln.SyscallConn()
  } else {
    err = errorf("invalid listener %+v", l)
  }

  fd := -1

  if err == nil {
    err = rconn.Control(func(fd_ uintptr) {
      fd = int(fd_)
    })
  }

  return fd, err
}


func isAddrInUse(err error) bool {
  return (
    err != nil &&
    strings.Index(err.Error(), "address already in use") != -1)
}

func isConnRefused(err error) bool {
  return (
    err != nil &&
    strings.Index(err.Error(), "connection refused") != -1)
}

func isUseOfClosedConn(err error) bool {
  return (
    err != nil &&
    strings.Index(err.Error(), "use of closed network connection") != -1)
}
