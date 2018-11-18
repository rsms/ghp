package main

import (
  "fmt"
  "io/ioutil"
  "path"
  "path/filepath"
  "runtime"
  "strings"
  "sync"
  "time"
)

type HelpersMap = map[string]interface{}


func NewHelpersMap(base HelpersMap) HelpersMap {
  h := make(HelpersMap)
  for k, v := range base {
    h[k] = v
  }
  return h
}


func cleanFileName(basedir, name string) string {
  var fn string
  if runtime.GOOS == "windows" {
    name = strings.Replace(name, "/", "\\", -1)
    fn = filepath.Join(basedir, strings.TrimLeft(name, "\\"))
  } else {
    fn = filepath.Join(basedir, strings.TrimLeft(name, "/"))
  }
  fn = filepath.Clean(fn)
  if !strings.HasPrefix(fn, basedir) {
    return ""
  }
  return fn
}


func buildBaseHelpers() HelpersMap {
  // helper functions shared by everything; all Ghp instances.
  h := make(HelpersMap)

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

  return h
}


func (g *Ghp) buildHelpers(base HelpersMap) HelpersMap {
  // helper functions shared by everything in the same Ghp instance.
  h := NewHelpersMap(base)

  // readfile reads a file relative to PubDir
  h["readfile"] = func (name string) (string, error) {
    fn := cleanFileName(g.config.PubDir, name)
    if fn == "" {
      return "", errorf("file not found %v", name)
    }
    data, err := ioutil.ReadFile(fn)
    if err != nil {
      return "", err
    }
    return string(data), nil
  }

  return h
}


func (c *PageCache) buildHelpers(base HelpersMap) HelpersMap {
  // Helper functions shared by all pages in the same PageCache.
  return base
}


func (p *Page) buildHelpers(base HelpersMap) HelpersMap {
  // Helper functions specific to this page.
  // When referencing page data, be careful to bind p to the function closures
  // instead of any data of p, as the buildHelpers function is called before
  // some of the page's data is configured.
  return base
}

// ------------------------------------------------------------

var (
  gBaseHelpersOnce sync.Once
  gBaseHelpers HelpersMap
)

func getBaseHelpers() HelpersMap {
  gBaseHelpersOnce.Do(func() {
    gBaseHelpers = buildBaseHelpers()
  })
  return gBaseHelpers
}
