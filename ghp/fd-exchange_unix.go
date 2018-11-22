package main

import (
  "net"
  "os"
  "syscall"
)

const fdExchangeMaxFDsPerMsg = 4
  // max number of fds to send in one message


// FdExchangeSendFiles sends files over conn.
//
func FdExchangeSendFiles(conn net.Conn, files... *os.File) error {
  fds := make([]int, len(files))
  for i := range files {
    fds[i] = int(files[i].Fd())  // int32 file descriptor
  }
  return FdExchangeSendFDs(conn, fds...)
}


// FdExchangeRecvFiles receives named files from conn.
// The order in which the files appeared in the sender's call to
// FdExchangeSend is mapped to the order of the names provided here.
//
func FdExchangeRecvFiles(conn net.Conn, names... string) ([]*os.File, error) {
  fds, err := FdExchangeRecvFDs(conn, len(names))
  if err != nil {
    return nil, err
  }

  files := make([]*os.File, 0, len(fds))
  for i, fd := range fds {
    files[i] = os.NewFile(uintptr(fd), names[i])
  }

  return files, err
}


// FdExchangeSendFDs sends file descriptors over conn.
// On Posix systems, conn must be of kind net.UnixConn.
//
func FdExchangeSendFDs(conn net.Conn, fds... int) error {
  // access system socket
  sockfile, err := conn.(*net.UnixConn).File()
  if err != nil {
    return err
  }
  defer sockfile.Close() // since net.UnixConn.File() returns a copy
  sockfd := int(sockfile.Fd())  // int32 file descriptor

  nremain := len(fds)
  n := fdExchangeMaxFDsPerMsg
  if nremain <= n {
    // common case
    oobbuf := syscall.UnixRights(fds...)
    return syscall.Sendmsg(sockfd, nil, oobbuf, nil, 0)
  }

  // send in chunks to avoid unknown system limits
  for {
    if n > nremain {
      n = nremain
    }
    oobbuf := syscall.UnixRights(fds[:n]...)
    if err := syscall.Sendmsg(sockfd, nil, oobbuf, nil, 0); err != nil {
      return err
    }
    if n == nremain {
      return nil
    }
    fds = fds[n:]
    nremain -= n
  }
}


// FdExchangeRecvFDs receives named file descriptors from conn.
// On Posix systems, conn must be of kind net.UnixConn.
// count defines the expected number of file descriptors, but the received
// amount might be smaller; i.e. len(r0)<=count
//
func FdExchangeRecvFDs(conn net.Conn, count int) ([]int, error) {
  // access system socket
  sockfile, err := conn.(*net.UnixConn).File()
  if err != nil {
    return nil, err
  }
  defer sockfile.Close() // since net.UnixConn.File() returns a copy
  sockfd := int(sockfile.Fd())  // int32 file descriptor

  fds := make([]int, 0, count)
  for count > 0 && err == nil {
    chunkcount := imin(count, fdExchangeMaxFDsPerMsg)
    fds, err = fdExchangeRecvFDs(fds, sockfd, chunkcount)
    count -= chunkcount
  }

  return fds, err
}


// fdExchangeRecvFDs is internal; used by FdExchangeRecvFDs
func fdExchangeRecvFDs(fds []int, sockfd, count int) ([]int, error) {
  // allocate 4 bytes per FD (int32) for out-of-band socket data
  oobbuf := make([]byte, syscall.CmsgSpace(count * 4))

  // read from socket into oobbuf
  _, _, _, _, err := syscall.Recvmsg(sockfd, nil, oobbuf, 0)
  if err != nil {
    return nil, errorf("syscall.Recvmsg: %v", err)
  }

  // parse messages in oobbuf
  mv, err := syscall.ParseSocketControlMessage(oobbuf)
  if err != nil {
    return nil, errorf("syscall.ParseSocketControlMessage %v", err)
  }

  // place all received file descriptors into one array
  for i := 0; i < len(mv); i++ {
    fds0, err := syscall.ParseUnixRights(&mv[i])
    if err != nil {
      return nil, errorf("syscall.ParseUnixRights %v", err)
    }
    fds = append(fds, fds0...)
  }

  return fds, err
}

