package main

import (
  "os"
  "sync"
)

var pageCache *PageCache


type PageCache struct {
  items     map[string]*Page  // keyed by filename
  itemsmu   sync.RWMutex

  buildq   map[string]chan *Page
  buildqmu  sync.Mutex
}


func NewPageCache() *PageCache {
  return &PageCache{
    items: make(map[string]*Page),
  }
}


// olderThanSource returns true if a page's source, or any of its parent
// sources, has been changed since the page or parent page was built.
//
func (p *Page) olderThanSource(d os.FileInfo) bool {
  if fileID(d) != p.fileid ||
     d.ModTime().UnixNano() > p.mtime ||
     len(p.relatedPageMissing) > 0 {
    // source has changed since page was built
    return true
  }

  // check parent
  if p.parent != nil {
    d, err := os.Stat(p.srcpath)
    return err != nil || p.parent.olderThanSource(d)
  }

  // Note: We could optionally use file-system observation and instead
  // mark sources changed as they change, instead of os.Stat on every
  // request. os.Stat should be very efficient though, so unclear if the
  // added complexity and sync locking of a file-system observer approach
  // would be much better.

  return false
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


// Build builds the page from source file f.
// This is concurrency-safe; multiple calls while a page is being built are
// all multiplexed to the same "build".
//
func (c *PageCache) Build(bc *buildCtx, f *os.File, d os.FileInfo) (*Page, error) {
  name := f.Name()

  c.buildqmu.Lock()

  if c.buildq == nil {
    c.buildq = make(map[string]chan *Page)
  } else if bch, ok := c.buildq[name]; ok {
    // already in progress of being built

    // done with buildq
    c.buildqmu.Unlock()

    // Wait for other goroutine who started the build
    p := <- bch
    
    return p, p.builderr
  }

  // If we get here, name was not found in buildq
  
  // Calling goroutine is responsible for building. Setup condition in build.
  bch := make(chan *Page)
  c.buildq[name] = bch
  c.buildqmu.Unlock()  // done with buildq

  // Build
  p := &Page{}
  p.buildSafe(bc, f, d)

  // Place result in items map (full write-lock)
  c.itemsmu.Lock()
  c.items[name] = p
  c.itemsmu.Unlock()

  // Clear buildq and send on chan
  c.buildqmu.Lock()

  // Send build page to anyone who is listening
  broadcast_loop:
  for {
    // logf("[pc] %v bch broadcast p", name)
    select {
    case bch <- p:
      break
    default:
      break broadcast_loop
    }
  }
  
  delete(c.buildq, name)
  c.buildqmu.Unlock()

  return p, p.builderr
}


func init() {
  pageCache = NewPageCache()
}
