package main

import (
  "net"
  "strings"
  "io"
  "os"
  "time"
  "path/filepath"
)

const (
  cmdTakeOver = "take-over"
  cmdOk = "ok"
  cmdError = "error"
)

// Zero-downtime restart
type Zdr struct {
  Shutdown   func()  // Called to gracefully shut down the system
  
  sockpath   string
  masterln   net.Listener
  shutdownch chan error
}


func NewZdr(sockpath string) *Zdr {
  return &Zdr{
    sockpath: sockpath,
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
func (z *Zdr) AcquireMasterRole(timeout time.Duration) error {
  if z.Shutdown == nil {
    return errorf("[zdr] .Shutdown is nil")
  }

  listenRetried := false

  retry_acquireMasterRole:
  err := z.acquireMasterRole()
  if err != nil && isAddrInUse(err) {
    // address in use

    logf("[zdr] taking over master role...")

    // This means that either another instance is running and is listening,
    // or that a previous instance crached and left the socket dangling.
    //
    // In any case, we first try to connect to the socket.
    conn, err := net.Dial("unix", z.sockpath)

    if err != nil {
      if !listenRetried && strings.Index(err.Error(), "connection refused") != -1 {
        // Probably left-over socket -- remove and try again to listen
        os.Remove(z.sockpath)
        listenRetried = true
        goto retry_acquireMasterRole
      }
      return err
    }

    // Take over
    if timeout > 0 {
      conn.SetReadDeadline(time.Now().Add(timeout))
    }
    err = z.takeOverMasterRole(conn)
    if err != nil {
      return err
    }

    // Take-over succeeded and we should now be able to listen
    listenRetried = false // allow retry
    goto retry_acquireMasterRole
  }

  return err
}


// Shutdown gives up the master role.
// Calls z.Shutdown followed by closing the socket listener.
//
func (z *Zdr) shutdown() error {
  if z.masterln == nil {
    return errorf("does not have master role")
  }

  var err error

  // Call z.Shutdown which will block until shutdown has completed
  func() {
    defer func() {
      if r := recover(); r != nil {
        err = errorf("panic in z.Shutdown: %v", r)
      }
    }()
    z.Shutdown()
  }()

  z.shutdownch <- err

  return nil
}


func (z *Zdr) Close() {
  if z.masterln != nil {
    z.masterln.Close()
    z.masterln = nil
    // os.Remove(z.sockpath)
  }
}


func (z *Zdr) masterLoop() {
  defer z.masterln.Close()

  for {
    // Handle each connection in a serial manner to avoid race conditions.
    conn, err := z.masterln.Accept()
    if err != nil {
      logf("[zdr] error in Accept: %v", err)
      // TODO: handle "use of closed network connection" (when terminating app)
      // TODO: call Listen?
      return
    }

    defer conn.Close()

    // logf("masterLoop: %p connected", conn)

    // communicate serially
    mr := NewIpcMsgReader(conn)
    mw := NewIpcMsgWriter(conn)
    for {
      // read next message
      msg, err := mr.Read()

      // Handle read error
      if err != nil {
        conn.Close()
        if err == io.EOF {
          logf("[zdr] %p disconnected", conn)
        } else {
          logf("[zdr] %p error %v (closing connection)", conn, err)
        }
        break
      }

      if msg.Cmd == cmdTakeOver {
        // The requestor wants to take over the master role.
        // Give up the master role.
        if err := z.shutdown(); err != nil {
          // Reply to requestor if there was an error with shutdown
          mw.Write(NewIpcMsg(cmdError, err.Error()))
        }

        // Close listener to signal OK
        z.Close()
        conn.Close()

        return  // end masterLoop
      } else {
        // log and ignore unexpected messages
        logf("[zdr] masterLoop received unexpected message %+v", msg)
      }
    } // message read loop

  }
}


func (z *Zdr) takeOverMasterRole(conn net.Conn) error {
  defer conn.Close()

  mw := NewIpcMsgWriter(conn)
  mr := NewIpcMsgReader(conn)

  // send request to take over the role as master
  err := mw.Write(NewIpcMsg(cmdTakeOver))
  if err != nil {
    return err
  }

  // wait for reply
  m, err := mr.Read()
  if err != nil {
    // success
    // Note: We primarily care about err == io.EOF, however a broken pipe
    // or any other communication error is assumed to mean a success since
    // the other ends closes the connection (reliquished it.)
    return nil
  }

  // read reply
  errmsg := "?"
  if m.Cmd == cmdError {
    if len(m.Args) > 0 {
      errmsg = m.Args[0]
    }
  } else {
    errmsg = "unexpected ipcmsg " + m.Cmd
  }
  return errorf("other program failed to relinquish master role: %s", errmsg)
}


func (z *Zdr) acquireMasterRole() error {
  retried := false
  
  listen_:
  // logf("try acquire master role...")
  ln, err := net.Listen("unix", z.sockpath)

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

  // setup shutdown channel for whenever we shut down
  z.shutdownch = make(chan error, 1)

  // dispatch master loop (reads and writes messages)
  go z.masterLoop()

  return nil
}


func isAddrInUse(err error) bool {
  return strings.Index(err.Error(), "address already in use") != -1
}

