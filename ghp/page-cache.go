package main

import (
  "os"
  "sync"
  "strings"
)


type PageCache struct {
  g       *Ghp
  c       *PagesConfig
  fileext string
  srcdir  string

  items   map[string]*Page  // keyed by filename
  itemsmu sync.RWMutex

  buildq   map[string]chan *Page
  buildqmu sync.Mutex

  helpers  HelpersMap
}


func NewPageCache(g *Ghp, config *PagesConfig) *PageCache {
  fileext := ".ghp"
  if config.FileExt != "" {
    // make sure it begins with a single "."
    fileext = "." + strings.TrimLeft(config.FileExt, ".")
  }

  c := &PageCache{
    g: g,
    c: config,
    fileext: fileext,
    srcdir: g.config.PubDir,
    items: make(map[string]*Page),
  }

  // build helper functions
  c.helpers = c.buildHelpers(g.helperfuns)

  return c
}


// Get returns a Page for the provided source file.
// The page is either fetched from cache or built from source, depending
// on if it's cached and if the cached version is up-to date compared to
// the source file's modification timestamp.
//
func (c *PageCache) Get(bc *buildCtx, f *os.File, d os.FileInfo) (*Page, error) {
  filename := f.Name()

  if p := c.GetCached(filename); p != nil && !p.olderThanSource(d) {
    // up-to date page found in cache
    return p, p.builderr
  }

  return c.Build(bc, f, d)
}


// GetCached unconditionally returns a page if one is found in cache.
// Caller should check p.builderr
//
func (c *PageCache) GetCached(name string) *Page {
  c.itemsmu.RLock()
  p := c.items[name]
  c.itemsmu.RUnlock()
  return p
}

