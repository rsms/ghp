package main

import (
  "fmt"
  "go/build"
  "go/parser"
  "go/token"
  "io/ioutil"
  "log"
  "os"
  "path/filepath"
  "strings"
  "sync"
  "sync/atomic"
  "time"
  "github.com/fsnotify/fsnotify"
)

// --------------------------------------------------------------------------

type SrcFile struct {
  name   string
  mtime  int64        // modification unix timestamp (nanoseconds)
  pkg    *SrcPackage  // package this file belongs to
}


func (f *SrcFile) updateMtime(mtime int64) bool {
  return atomicStoreMaxInt64(&f.mtime, mtime)
}

// --------------------------------------------------------------------------

type SrcPackage struct {
  dir         string
  srcmtime    int64          // source max timestamp (nanoseconds)
  depmtime    int64          // dependencies' max timestamp (nanoseconds)
  nfiles      int32          // number of source files
  dependants  []*SrcPackage  // other packages that import this package
  depmu       sync.RWMutex   // lock for dependants
}


// NumFiles returns the number of source files that comprises the package
//
func (p *SrcPackage) NumFiles() int32 {
  return atomic.LoadInt32(&p.nfiles)
}


// ModTimestamp returns a unix timestamp in nanoseconds of the most recent
// modification, including dependencies.
//
func (p *SrcPackage) ModTimestamp() int64 {
  srcmtime := atomic.LoadInt64(&p.srcmtime)
  depmtime := atomic.LoadInt64(&p.depmtime)
  if srcmtime > depmtime {
    return srcmtime
  }
  return depmtime
}


func (dependant *SrcPackage) addDep(dependency *SrcPackage) {
  // p depends on p2 -- register p as dependant of p2
  dependency.depmu.Lock()
  dependency.dependants = append(dependency.dependants, dependant)
  dependency.depmu.Unlock()

  // update dependant.depmtime to be at least as late as dependency's
  // srcmtime or depmtime
  dependant.updateDepmtime(dependency.ModTimestamp())
}


func (p *SrcPackage) maybeAddDep(dependency *SrcPackage) bool {
  if dependency.dependants != nil {
    dependency.depmu.RLock()
    for _, p2 := range dependency.dependants {
      if p2 == p {
        // already registered
        dependency.depmu.RUnlock()
        return false
      }
    }
    dependency.depmu.RUnlock()
  }
  // not registered -- add
  p.addDep(dependency)
  return true
}


func (p *SrcPackage) updateSrcmtime(mtime int64) bool {
  return atomicStoreMaxInt64(&p.srcmtime, mtime)
}


func (p *SrcPackage) updateDepmtime(mtime int64) bool {
  return atomicStoreMaxInt64(&p.depmtime, mtime)
}

// --------------------------------------------------------------------------

type SrcGraph struct {
  rootdir  string
  mtime    int64        // modification unix timestamp (nanoseconds)
  mainpkg  *SrcPackage

  mu       sync.RWMutex // RLock when traversing the graph, Lock when editing

  pkgmap   *sync.Map    // SrcPackage.dir => *SrcPackage
  filemap  *sync.Map    // SrcFile.name => *SrcFile

  fswatcher  *fsnotify.Watcher
  fswstopch  chan interface{}
}


func NewSrcGraph(rootdir string) *SrcGraph {
  return &SrcGraph{
    rootdir: rootdir,
  }
}


func (g *SrcGraph) Scan() error {
  g.mu.Lock()
  defer g.mu.Unlock()

  g.mainpkg = nil
  g.pkgmap = &sync.Map{}
  g.filemap = &sync.Map{}

  pkg, err := build.Default.ImportDir(g.rootdir, 0)
  if err != nil {
    return err
  }

  p, err := g.scanAddPkg(pkg, ".")
  if err != nil {
    return err
  }

  g.mainpkg = p

  // watch the file system
  if err = g.initFSWatcher(); err != nil {
    return err
  }

  // print some info on packages
  g.logInfo()
  log.Printf("[sg] mtime: %v\n", g.mtime)

  return nil
}


func (g *SrcGraph) updateMtime(mtime int64) bool {
  return atomicStoreMaxInt64(&g.mtime, mtime)
}


// GetPackage retrieves SrcPackage object for dir.
// Returns nil if there's no knowledge of a package at dir.
// dir should be relative and rooted in g.rootdir
//
func (g *SrcGraph) GetPackage(dir string) *SrcPackage {
  // Note: no locking needed
  pkgmap := g.pkgmap
  if pkgmap != nil {
    pobj, ok := pkgmap.Load(dir)
    if ok {
      return pobj.(*SrcPackage)
    }
  }
  return nil
}


func (g *SrcGraph) initFSWatcher() error {
  if g.fswatcher != nil {
    g.stopFSWatcher()
  }

  fswatcher, err := fsnotify.NewWatcher()
  if err != nil {
    return err
  }
  
  g.fswstopch = make(chan interface{})
  g.fswatcher = fswatcher

  // watch directories of packages
  g.pkgmap.Range(func(k, v interface{}) bool {
    p := v.(*SrcPackage)
    absdir := filepath.Join(g.rootdir, p.dir)
    if err := g.fswatcher.Add(absdir); err != nil {
      // print error and continue range iteration
      log.Printf("[sg/fswatcher] failed to watch %q: %s", absdir, err.Error())
    } else {
      log.Printf("[sg/fswatcher] watching %q", absdir)
    }
    return true
  })

  // start the file system watcher goroutine
  go g.fswatcherEventLoop()

  return nil
}


func (g *SrcGraph) stopFSWatcher() {
  assert(g.fswatcher != nil)
  g.fswstopch <- struct{}{}
  g.fswatcher.Close()
  g.fswatcher = nil
  g.fswstopch = nil
}


func (g *SrcGraph) logInfo() {
  print("packages:\n")
  g.pkgmap.Range(func(k, v interface{}) bool {
    p := v.(*SrcPackage)
    fmt.Printf("  %q (srcmtime: %+v, depmtime: %+v",
      p.dir, p.srcmtime, p.depmtime)
    p.depmu.RLock()
    if len(p.dependants) > 0 {
      for i, dep := range p.dependants {
        if i == 0 {
          fmt.Printf(", dependants: %q", dep.dir)
        } else {
          fmt.Printf(", %q", dep.dir)
        }
      }
    }
    p.depmu.RUnlock()
    print(")\n")
    return true
  })
  print("files:\n")
  g.filemap.Range(func(relname, v interface{}) bool {
    f := v.(*SrcFile)
    fmt.Printf("  %q (mtime: %+v, pkg: %q)\n", relname, f.mtime, f.pkg.dir)
    return true
  })
}


func (g *SrcGraph) scanImport(path, parentDir string) (*SrcPackage, error) {
  pkg, err := build.Default.Import(path, parentDir, build.FindOnly)
  if err != nil {
    return nil, err
  }

  if !strings.HasPrefix(pkg.Dir, g.rootdir) {
    // outside package (not tracked)
    log.Printf("[sg/scanImport] ignore outside pkg %q", pkg.Dir)
    return nil, nil
  }

  reldir := pkg.Dir[len(g.rootdir)+1:]

  if pobj, ok := g.pkgmap.Load(reldir); ok {
    log.Printf("[sg/scanImport] ret already-scanned pkg %q", reldir)
    return pobj.(*SrcPackage), nil
  }

  pkg, err = build.Default.ImportDir(pkg.Dir, 0)
  if err != nil {
    return nil, err
  }

  return g.scanAddPkg(pkg, reldir)
}


func (g *SrcGraph) scanAddPkg(pkg *build.Package, dir string) (*SrcPackage, error) {
  p := &SrcPackage{
    dir: dir,
  }

  g.pkgmap.Store(p.dir, p)

  for _, impath := range pkg.Imports {
    if pathIsDotRelative(impath) {
      p2, err := g.scanImport(impath, pkg.Dir)
      if err != nil {
        return nil, err
      }
      if p2 != nil {
        p.addDep(p2)
      }
    }
  }

  // register source files
  for _, name := range pkg.GoFiles {
    relname := filepath.Join(p.dir, name)
    f, _, err := g.addSrcFile(p, relname)
    if err != nil {
      return nil, err
    }

    // maybe update package's mtime
    if f.mtime > p.srcmtime {
      p.srcmtime = f.mtime
    }
  }

  // maybe update graph's mtime
  if p.srcmtime > g.mtime {
    g.mtime = p.srcmtime
  }

  return p, nil
}


// the second return value indicates if this was the first file added to
// the package or not.
//
func (g *SrcGraph) addSrcFile(p *SrcPackage, name string) (*SrcFile, bool, error) {
  d, err := os.Stat(filepath.Join(g.rootdir, name))
  if err != nil {
    return nil, false, err
  }

  f := &SrcFile{
    name:  name,
    mtime: d.ModTime().UnixNano(),
    pkg:   p,
  }

  log.Printf("[sg/pkg %q] srcfile %q %+v\n", p.dir, name, f)
  g.filemap.Store(name, f)

  var firstfile bool
  if atomic.AddInt32(&p.nfiles, 1) == 1 {
    firstfile = true
  }

  return f, firstfile, nil
}


func (g *SrcGraph) onGraphModified(mtime int64) {
  if g.updateMtime(mtime) {
    log.Printf("[sg] graph modified; mtime %v", mtime)
  }
}


// func (g *SrcGraph) updateScanPkg(pkg *SrcPackage) {
//   reldir := pkg.Dir[len(g.rootdir)+1:]

//   if pobj, ok := g.pkgmap.Load(reldir); ok {
//     log.Printf("[srcgraph/scanImport] ret already-scanned pkg %q", reldir)
//     return pobj.(*SrcPackage), nil
//   }

//   pkg, err = build.Default.ImportDir(pkg.Dir, 0)
//   if err != nil {
//     return nil, err
//   }
// }


func (g *SrcGraph) updateScanFile(f *SrcFile) error {
  path := filepath.Join(g.rootdir, f.name)
  src, err := ioutil.ReadFile(path)
  if err != nil {
    return err
  }

  fset := token.NewFileSet()
  a, err := parser.ParseFile(fset, f.name, src, parser.ImportsOnly)
  if err != nil {
    return err
  }

  // scan file imports
  fmt.Printf("[sg/updateScanFile] packages imported by %q:\n", f.name)
  for _, s := range a.Imports {
    impath := s.Path.Value[1:len(s.Path.Value)-1] // strip enclosing ""

    if !pathIsDotRelative(impath) {
      // ignore non-relative imports
      continue
    }

    fmt.Printf("- import %q\n", impath)

    pkgdir := filepath.Join(f.pkg.dir, impath)

    if deppkg := g.GetPackage(pkgdir); deppkg != nil {
      // known, existing package -- make sure dependency is registered
      f.pkg.maybeAddDep(deppkg)
      continue
    }

    fmt.Printf("  - unregistered package %q\n", pkgdir)

    // absdir := filepath.Join(g.rootdir, imppkgdir)
    // if err := g.fswatcher.Add(absdir); err != nil {
    // }
  }

  return nil
}


func (g *SrcGraph) onPackageSrcModified(p *SrcPackage, mtime int64) {
  if p.updateSrcmtime(mtime) {
    log.Printf("[sg] package %q modified; mtime %v", p.dir, mtime)
    // g.updateScanPkg(p)
    g.onGraphModified(mtime)
  }
}


func (g *SrcGraph) onPackageEmptied(p *SrcPackage, mtime int64) {
  log.Printf("[sg] package %q was emptied", p.dir)
  g.onPackageSrcModified(p, mtime)
  // Maybe just do nothing?
  // For instance, it's very possible that with a single-file package, when
  // the file is renamed, we first get a "REMOVED" or "RENAMED" event from
  // the OS, which would trigger this function, and then immediately a
  // "CREATE" event would follow to signal the new name of the file, which
  // we interprets as "new file appeared" (since fs events are relatively
  // unreliable.) If we were to remove the package when that happens, we'd
  // lose the package in our registry.
  //
  // So what we could do is to check 
}


func (g *SrcGraph) onPackageResuscitated(p *SrcPackage, mtime int64) {
  log.Printf("[sg] package %q was resuscitated", p.dir)
  g.onPackageSrcModified(p, mtime)
}


func (g *SrcGraph) onFileModified(f *SrcFile, mtime int64) {
  if f.updateMtime(mtime) {
    log.Printf("[sg] file %q modified; mtime %v", f.name, mtime)
    err := g.updateScanFile(f)
    if err != nil {
      log.Printf("[sg] updateScanFile failed for %q: %s", f.name, err.Error())
    }
    g.onPackageSrcModified(f.pkg, mtime)
  }
}


func (g *SrcGraph) onFileDisappeared(f *SrcFile, relname string) {
  log.Printf("[sg] file %q disappeared", f.name)
  pkg := f.pkg

  g.filemap.Delete(relname)

  mtime := time.Now().UnixNano()

  if atomic.AddInt32(&pkg.nfiles, -1) == 0 {
    // last file of the package removed
    g.onPackageEmptied(pkg, mtime)
  } else {
    // still some source files in package -- mark it as modified
    g.onPackageSrcModified(pkg, mtime)
  }
}



func (g *SrcGraph) handleFSEvent(e *fsnotify.Event) {

  if e.Op == fsnotify.Chmod {
    // ignore CHMOD-only events
    return
  }

  if !strings.HasPrefix(e.Name, g.rootdir) {
    log.Printf("[sg/fswatch] unexpected file outside rootdir %q", e.Name)
    return
  }

  name := e.Name[len(g.rootdir)+1:]
  fmt.Printf("%- 6s %s\n", e.Op.String(), name)

  // look up file
  v, ok := g.filemap.Load(name)
  if ok {
    f := v.(*SrcFile)

    if e.Op & fsnotify.Write == fsnotify.Write {
      // file was modified
      if d, err := os.Stat(e.Name); err == nil {
        g.onFileModified(f, d.ModTime().UnixNano())
      } else {
        // treat this condition as file disappeared since stat failed
        log.Printf("[sg/fswatch] stat failed after event %s on %q: %s",
          e.Op.String(), e.Name, err.Error())
        g.onFileDisappeared(f, name)
      }

    } else if e.Op & fsnotify.Remove == fsnotify.Remove || e.Op & fsnotify.Rename == fsnotify.Rename {
      // fsnotify.Rename is essentially "file moved somewhere" which includes
      // files being moved to Trash.
      //
      // event.Op may have multiple event flags set when multiple flags makes
      // sense to describe the event. For instance, when moving a directory,
      // REMOVE|RENAME maybe be both set to signal that
      // "directory disappeared because it was renamed" (while just REMOVE
      // might signal that the directory was deleted.) This may very well
      // vary between platforms and OS versions. Yay.
      //
      // Based on these observations, we treat Rename and Remove the same
      // way, which is semantically closer to "remove" (file disappeared.)
      g.onFileDisappeared(f, name)
    }
  } else if e.Op & fsnotify.Create == fsnotify.Create || e.Op & fsnotify.Write == fsnotify.Write {
    // file appeared (not found in g.filemap and event==CREATE)
    match, err := build.Default.MatchFile(filepath.Dir(e.Name), filepath.Base(e.Name))
    if match && err == nil {
      // file is a valid go source file
      pkgdir := filepath.Dir(name)
      pkgv, ok := g.pkgmap.Load(pkgdir)
      if ok {
        pkg := pkgv.(*SrcPackage)
        // file added to existing package
        f, firstfile, err := g.addSrcFile(pkg, name)
        if err != nil {
          log.Printf("[sg/fswatch] file reg error %q: %s", name, err.Error())
        } else if firstfile {
          g.onPackageResuscitated(pkg, f.mtime)
        } else {
          g.onPackageSrcModified(pkg, f.mtime)
        }
      // } else {
      //   // Maybe call some function similar to scanImport and then
      //   // g.scanAddPkg() to register this new package.
      //   //
      //   // However, we may simply dirty the graph here since if another package
      //   // depends on this potentially new one, that existing other one will
      //   // be edited (and we will update its mtime.)
      }
    // } else {
    //   fmt.Printf("build.Default.MatchFile(%q, %q) => false (err=%+v)\n",
    //     filepath.Dir(e.Name), filepath.Base(e.Name), err)
    }
  }
  // else: change to unrelated file -- ignore

  // These events are pretty unreliable in terms of specificity.
  // Here's what happens on macOS 10.13.6 (Darwin 17.7.0):
  //
  // -- edit index.go
  // WRITE  example/index.go
  // WRITE  example/index.go
  // WRITE  example/index.go
  // WRITE  example/index.go
  // WRITE  example/index.go
  // -- add untitled.go
  // CREATE example/untitled.go
  // -- edit untitled.go
  // WRITE  example/untitled.go
  // -- rename untitled.go -> foo.go
  // CREATE example/foo.go
  // RENAME example/untitled.go
  // -- delete foo.go
  // RENAME example/foo.go
  // WRITE  example/.DS_Store
  // WRITE  example/.DS_Store
}


func (g *SrcGraph) fswatcherEventLoop() {
  w := g.fswatcher
  stopch := g.fswstopch

  loop0:
  for {
    select {
    case event := <-w.Events:
      // check if the watcher being processed by this goroutine is the
      // same watcher that is currently the "main" watcher. Else simply
      // do nothing.
      if w == g.fswatcher {
        g.handleFSEvent(&event)
      }
    case err := <-w.Errors:
      log.Println("[sg/fswatch] error:", err)
      break loop0
    case <-stopch:
      log.Print("[sg/fswatch] event loop stopped")
      return
    }
  }
  // stop here after an error occured, waiting for a call to stopFSWatcher()
  <-stopch
  log.Print("[sg/fswatch] event loop stopped")
}



// build.Package (https://golang.org/pkg/go/build/#Package)
// {
//   Dir: "/Users/rsms/src/ghp/pub/example"
//   Name: "main"
//   ImportPath: "."
//   Root:
//   SrcRoot:
//   PkgRoot:
//   PkgTargetRoot:
//   BinDir:
//   Goroot:false
//   PkgObj:
//   AllTags:[]
//   ConflictDir:
//   BinaryOnly:false
//   GoFiles:[index.go]
//   CgoFiles:[]
//   IgnoredGoFiles:[]
//   InvalidGoFiles:[]
//   CFiles:[]
//   CXXFiles:[]
//   MFiles:[]
//   HFiles:[]
//   FFiles:[]
//   SFiles:[]
//   SwigFiles:[]
//   SwigCXXFiles:[]
//   SysoFiles:[]
//   // ...
//   Imports: [./foo, ghp]
//   ImportPos: map[
//     "ghp": [/Users/rsms/src/ghp/pub/example/index.go:4:3]
//     "./foo": [/Users/rsms/src/ghp/pub/example/index.go:5:3]
//   ]
//   // ...
// }



// err := filepath.Walk(rootdir, func(path string, d os.FileInfo, err error) error {
//   if err != nil {
//     return err
//   }
//   log.Printf("path=%q, d.Name()=%q\n", path, d.Name())

//   if d.IsDir() {
//     return filepath.SkipDir
//   }

//   if strings.HasSuffix(d.Name(), ".go") {
//     nsrcfiles++
//     mtime := d.ModTime()
//     if mtime.After(maxtime) {
//       maxtime = mtime
//     }
//     return filepath.SkipDir
//   }
//   return nil
// })

// // return error if we didn't find any .go files
// if err == nil && nsrcfiles == 0 {
//   err = fmt.Errorf("no source files found in %q", rootdir)
// }

