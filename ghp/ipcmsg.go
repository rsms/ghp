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

func (w *IpcMsgWriter) Write(m interface{}) error {
  return w.Encoder.Encode(m)
}



type IpcMsgReader struct {
  *gob.Decoder
}

func NewIpcMsgReader(r io.Reader) *IpcMsgReader {
  return &IpcMsgReader{ Decoder: gob.NewDecoder(r) }
}

func (r IpcMsgReader) Read() (*IpcMsg, error) {
  var m IpcMsg
  err := r.Decoder.Decode(&m)
  return &m, err
}
