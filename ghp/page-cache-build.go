package main

import (
  "os"
  "path/filepath"
  "runtime/debug"
  "strings"
  "time"
  "syscall"
  tparse "text/template/parse"
)


// buildCtx is used for the build process to keep track of things.
// One buildCtx is used per call stack, and never crosses goroutines.
// Not thread-safe.
//
type buildCtx struct {
  building map[string]bool
}

func (bc *buildCtx) IsBuilding(name string) bool {
  return bc.building != nil && bc.building[name]
}

func (bc *buildCtx) SetIsBuilding(name string) {
  if bc.building == nil {
    bc.building = make(map[string]bool)
  } else if bc.building[name] {
    panic(errorf("already building %v", name))
  }
  bc.building[name] = true
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
  p := &Page{ cache: c }
  c.buildSafe(bc, p, f, d)

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


// buildPageSafe wraps buildPage and is guaranteed never to panic
// and always returns a non-nil Page struct.
// On panic or error, .builderr will be set in the returned Page.
//
func (c *PageCache) buildSafe(bc *buildCtx, p *Page, f *os.File, d os.FileInfo) {
  defer func() {
    if r := recover(); r != nil {
      logf("panic in Page.build: %v", r)
      debug.PrintStack()
      p.mtime = time.Now().UnixNano()
      p.builderr = errorf("%v", r)
    }
  }()
  err := c.buildPage(bc, p, f, d)
  if err != nil {
    p.mtime = time.Now().UnixNano()
  }
}


// buildPage builds a page from a source file.
//
func (c *PageCache) buildPage(bc *buildCtx, p *Page, f *os.File, d os.FileInfo) error {
  name := f.Name()

  // mark source file as being in the process of building
  bc.SetIsBuilding(name)

  // friendly name is name relative to pubdir
  name = relfile(c.srcdir, name)
  mtime := time.Now().UnixNano()

  logf("build %q", name)

  // read file contents
  source, err := freadStr(f, d.Size())
  if err != nil {
    p.builderr = err
    return err
  }

  // build page helper functions, based on c.helpers
  helpers := p.buildHelpers(c.helpers)

  // parse source
  asts, meta, err := c.parsePage(name, source, helpers)
  if err != nil {
    p.builderr = err
    return err
  }

  // init Page
  p.srcpath = f.Name()
  p.name = name
  p.mtime = mtime
  p.meta = meta
  p.relatedPageMissing = ""

  // set fileid
  p.fileid = fileID(d)

  // has parent?
  if meta != nil && len(meta.Parent) > 0 {
    pp, err := c.loadRelatedPage(bc, f.Name(), meta.Parent)
    if err != nil {
      if os.IsNotExist(err) {
        err = errorf("parent not found %q", meta.Parent)
      }
      p.relatedPageMissing = meta.Parent
      p.builderr = err
      return err
    }
    p.parent = pp
  }

  // html or text template?
  if meta == nil ||
     meta.Type == "" ||
     strings.Index(meta.Type, "html") != -1 ||
     strings.Index(meta.Type, "xml") != -1 {
    p.t = NewHtmlTemplate(p.name)
  } else {
    p.t = NewTextTemplate(p.name)
  }

  // add primary template
  p.t, err = p.t.AddParseTree1(p.name, asts[p.name])
  if err != nil {
    p.builderr = err
    return err
  }

  // add parent 
  if p.parent != nil {
    for _, pt := range p.parent.t.Templates() {
      logf("add branch template: %v", pt.Name())
      if err := p.t.AddParseTree(pt.Name(), pt.Tree()); err != nil {
        p.builderr = err
        return err
      }
    }
  }

  // add any additional templates defined by the source file
  for tname, ast := range asts {
    // creates or replaces template with tname in t
    if tname != p.name {
      // Returns t if tname == t.Name(), else the named template is returned.
      logf("add leaf template: %v", tname)
      if err := p.t.AddParseTree(tname, ast); err != nil {
        p.builderr = err
        return err
      }
    }
  }

  p.t.Option("missingkey=zero")
  p.t.Funcs(helpers)

  p.builderr = nil
  return nil
}


func (c *PageCache) loadRelatedPage(bc *buildCtx, basename, name string) (*Page, error) {
  var err error
  name, err = c.relatedFilename(basename, name)
  if err != nil {
    return nil, err
  }

  if bc.IsBuilding(name) {
    // relationship cycle
    return nil, errorf(
      "cyclic relationship %v -- %v",
      relfile(c.srcdir, basename),
      relfile(c.srcdir, name),
    )
  }

  // open for reading
  f, err := os.Open(name)
  if err != nil {
    return nil, err
  }
  defer f.Close()

  // stat
  d, err := f.Stat()
  if err != nil {
    return nil, err
  }

  // load
  return c.Get(bc, f, d)
}


func (c *PageCache) relatedFilename(basename, othername string) (string, error) {
  var fn string
  if othername == "" {
    return fn, errorf("empty filename")
  }
  if othername[0] == '/' {
    // absolute filename is rooted in pubdir
    fn = pjoin(c.srcdir, strings.TrimLeft(othername, "/"))
  } else {
    // relative to current file
    fn = pjoin(basename, "..", othername)
  }
  fn = filepath.Clean(fn)
  if !strings.HasPrefix(fn, c.srcdir) {
    return "", errorf("file not found %v", othername)
  }
  return fn, nil
}


func (c *PageCache) parsePage(name, source string, helpers HelpersMap) (map[string]*tparse.Tree, *PageMetadata, error) {
  // parse meta, if any
  meta, metaEndPos, err := parsePageMetadata(name, source)
  if err != nil {
    return nil, nil, err
  }
  // trim away meta from source
  source2 := source[metaEndPos:]

  // logf("meta: %+v", meta)

  // template delimiters
  delimL := "{"
  delimR := "}"
  if meta != nil && len(meta.TemplateDelims) > 0 {
    if len(meta.TemplateDelims) != 2 {
      return nil, meta, errorf(
        "incorrect template metadata: template/delimiters should be a list of exactly two strings")
    }
    delimL = meta.TemplateDelims[0]
    delimR = meta.TemplateDelims[1]
  }

  // parse go template
  asts, err := tparse.Parse(name, source2, delimL, delimR, helpers)
  if err != nil {
    // re-parse with dummy blank lines added so that source line matches
    nlines := countByte(source[:metaEndPos+1], '\n')
    source2 = strings.Repeat("\n", nlines) + source2
    _, err = tparse.Parse(name, source2, "{", "}", helpers)
  }

  return asts, meta, err
}


func fileID(d os.FileInfo) uint64 {
  if stat, ok := d.Sys().(*syscall.Stat_t); ok {
    return stat.Ino
  }
  return uint64(d.Size())
}

