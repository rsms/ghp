package main

import (
  "time"
  "path"
  "path/filepath"
  "io/ioutil"
  "runtime"
  "strings"
  "fmt"
)

var basePageHelpers map[string]interface{}


func cleanFileName(name string) string {
  var fn string
  if runtime.GOOS == "windows" {
    name = strings.Replace(name, "/", "\\", -1)
    fn = filepath.Join(config.PubDir, strings.TrimLeft(name, "\\"))
  } else {
    fn = filepath.Join(config.PubDir, strings.TrimLeft(name, "/"))
  }
  fn = filepath.Clean(fn)
  if !strings.HasPrefix(fn, config.PubDir) {
    return ""
  }
  return fn
}


func h_readfile(name string) (string, error) {
  fn := cleanFileName(name)
  if fn == "" {
    return "", errorf("file not found %v", name)
  }

  data, err := ioutil.ReadFile(fn)
  if err != nil {
    return "", err
  }
  return string(data), nil
}


func init() {
  // setup basic page helper functions
  h := make(map[string]interface{})

  h["now"] = func () time.Time {
    return time.Now()
  }

  h["cat"] = func (args... interface{}) string {
    var b strings.Builder
    fmt.Fprint(&b, args...)
    return b.String()
  }

  h["url"] = func (args... string) string {
    return path.Join(args...)
  }

  h["timestamp"] = func (v... interface{}) int64 {
    if len(v) == 0 {
      return time.Now().UTC().Unix()
    } else {
      if t, ok := v[0].(time.Time); ok {
        return t.UTC().Unix()
      }
    }
    return 0
  }

  h["readfile"] = h_readfile

  basePageHelpers = h
}
