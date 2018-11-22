package main

import (
  "io"
  "encoding/gob"
)


type IpcMsg struct {
  Cmd  string
  Args []string
}

func NewIpcMsg(cmd string, arg... string) *IpcMsg {
  return &IpcMsg{ Cmd: cmd, Args: arg }
}



type IpcMsgWriter struct {
  *gob.Encoder
}

func NewIpcMsgWriter(w io.Writer) *IpcMsgWriter {
  return &IpcMsgWriter{ Encoder: gob.NewEncoder(w) }
}

func (w *IpcMsgWriter) Write(cmd string, arg... string) error {
  m := IpcMsg{ Cmd: cmd, Args: arg }
  return w.WriteMsg(&m)
}

func (w *IpcMsgWriter) WriteMsg(m interface{}) error {
  return w.Encoder.Encode(m)
}



type IpcMsgReader struct {
  *gob.Decoder
}

func NewIpcMsgReader(r io.Reader) *IpcMsgReader {
  return &IpcMsgReader{ Decoder: gob.NewDecoder(r) }
}

func (r IpcMsgReader) Read(cmd string) (*IpcMsg, error) {
  var m IpcMsg
  err := r.ReadMsg(&m)
  if err == nil && m.Cmd != cmd {
    err = errorf("unexpected ipc message %q (expected %q)", m.Cmd, cmd)
  }
  return &m, err
}

func (r IpcMsgReader) ReadMsg(m *IpcMsg) error {
  return r.Decoder.Decode(m)
}
