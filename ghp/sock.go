package main

import (
  "strings"
  "syscall"
)

type ConnSock struct {
  Fd    int
  Proto string  // e.g. "tcp" or "unix"
  Addr  string  // host:port e.g. "127.0.0.1:1234"
}

func ParseConnSock(s string) (*ConnSock, error) {
  // "proto:host:port"
  v := strings.SplitN(s, ":", 3)
  if len(v) != 3 {
    return nil, errorf("invalid format for ParseConnSock %q", s)
  }

  return &ConnSock{
    Proto: v[0],
    Addr: v[1] + ":" + v[2],
  }, nil
}


func (s *ConnSock) Close() error {
  fd := s.Fd
  if fd != 0 {
    s.Fd = 0
    return syscall.Close(fd)
  }
  return nil
}
