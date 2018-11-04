package main

import (
  "bytes"
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


func relatedFilename(basename, othername string) (string, error) {
  var fn string
  if othername == "" {
    return fn, errorf("empty filename")
  }
  if othername[0] == '/' {
    // absolute filename is rooted in pubdir
    fn = pjoin(config.PubDir, strings.TrimLeft(othername, "/"))
  } else {
    // relative to current file
    fn = pjoin(basename, "..", othername)
  }
  fn = filepath.Clean(fn)
  if !strings.HasPrefix(fn, config.PubDir) {
    return "", errorf("file not found %v", othername)
  }
  return fn, nil
}


func loadRelatedPage(bc *buildCtx, basename, name string) (*Page, error) {
  var err error
  name, err = relatedFilename(basename, name)
  if err != nil {
    return nil, err
  }

  if bc.IsBuilding(name) {
    // relationship cycle
    return nil, errorf(
      "cyclic relationship %v -- %v",
      friendlyFileName(basename),
      friendlyFileName(name),
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
  return pageCache.Get(bc, f, d)
}


func fileID(d os.FileInfo) uint64 {
  if stat, ok := d.Sys().(*syscall.Stat_t); ok {
    return stat.Ino
  }
  return uint64(d.Size())
}


// buildPage builds a page from a source file.
//
func (p *Page) build(bc *buildCtx, f *os.File, d os.FileInfo) error {
  name := f.Name()

  // mark source file as being in the process of building
  bc.SetIsBuilding(name)

  // friendly name is name relative to pubdir
  name = friendlyFileName(name)
  mtime := time.Now().UnixNano()

  logf("build %q", name)

  // read file contents
  source, err := freadStr(f, d.Size())
  if err != nil {
    p.builderr = err
    return err
  }

  // parse source
  asts, meta, err := parsePage(name, source)
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
    pp, err := loadRelatedPage(bc, f.Name(), meta.Parent)
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

  p.t.Funcs(basePageHelpers)
  p.t.Option("missingkey=zero")

  p.builderr = nil
  return nil
}


// buildPageSafe wraps buildPage and is guaranteed never to panic
// and always returns a non-nil Page struct.
// On panic or error, .builderr will be set in the returned Page.
//
func (p *Page) buildSafe(bc *buildCtx, f *os.File, d os.FileInfo) {
  defer func() {
    if r := recover(); r != nil {
      logf("panic in Page.build: %v", r)
      debug.PrintStack()
      p.mtime = time.Now().UnixNano()
      p.builderr = errorf("%v", r)
    }
  }()

  err := p.build(bc, f, d)

  if err != nil {
    p.mtime = time.Now().UnixNano()
  }
}


func parsePage(name string, source string) (map[string]*tparse.Tree, *PageMetadata, error) {
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
  asts, err := tparse.Parse(name, source2, delimL, delimR, basePageHelpers)
  if err != nil {
    // re-parse with dummy blank lines added so that source line matches
    nlines := countByte(source[:metaEndPos+1], '\n')
    source2 = strings.Repeat("\n", nlines) + source2
    _, err = tparse.Parse(name, source2, "{", "}", basePageHelpers)
  }

  return asts, meta, err
}


func freadStr(f *os.File, size int64) (string, error) {
  var buf bytes.Buffer
  if int64(int(size)) == size {
    // buf.Grow takes an int, not an int64
    buf.Grow(int(size))
  }
  _, err := buf.ReadFrom(f)
  return buf.String(), err
}


func friendlyFileName(fspath string) string {
  filename, err := filepath.Rel(config.PubDir, fspath)
  if err == nil {
    return filename
  }
  return fspath
}
