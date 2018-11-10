package main

import (
  "fmt"
)


func (s *Servlet) Build() error {
  logf("[servlet] building %s -> %q", s, s.libfile)

  g := NewGoTool(
    "build",
    "-buildmode=plugin",
    // "-gcflags", "-p " + libfile,
    "-ldflags", "-pluginpath=" + s.libfile,  // needed for uniqueness
    "-o", s.libfile,
  )

  logf("go tool: %+v", g)

  // set working directory to servlet's source directory
  g.Cmd.Dir = s.dir

  // run go build
  _, stderr, err := g.RunBufferedIO()
  if err != nil {
    logf("[servlet] go build failed: %s\n%s", err.Error(), stderr.String())
    return makeGoBuildError(
      fmt.Sprintf("failed to build servlet %q", s.name),
      s.dir,
      stderr.String(),
    )
  }

  return nil
}


/*func (c *ServletCache) buildServlet(s *Servlet, libfile string) error {
  srcdir := c.servletDir(s.name)
  // libfile := c.servletLibFile(s.name, s.version)
  // builddir := pjoin(c.builddir, s.name)

  logf("[servlet] building %s -> %q", s, libfile)

  // TODO: make-like individual-files build cache, where we only
  // rebuild the .go files that doesn't have corresponding up-to-date
  // .o files in a cache directory.

  // TODO: consider resolving "go" at startup and panic if it can't be found.

  // build servlet as plugin package
  cmd := exec.Command(
    "go", "build",
    "-buildmode=plugin",
    // "-gcflags", "-p " + libfile,
    "-ldflags", "-pluginpath=" + libfile,  // needed for uniqueness
    "-o", libfile,
  )

  // set working directory to servlet's source directory
  cmd.Dir = srcdir

  // set env
  cmd.Env = config.getGoProcEnv()

  // buffer output
  var outbuf bytes.Buffer
  var errbuf bytes.Buffer
  cmd.Stdout = &outbuf
  cmd.Stderr = &errbuf

  // execute program; errors if exit status != 0
  if err := cmd.Run(); err != nil {
    logf("[servlet] go build error: %s\n", err.Error())
    return makeGoBuildError(
      fmt.Sprintf("failed to build servlet %q", s.name),
      srcdir,
      errbuf.String(),
    )
  }

  // if outbuf.Len() > 0 {
  //   logf("stdout: %q\n", outbuf.String())
  // }
  // if errbuf.Len() > 0 {
  //   logf("stderr: %q\n", errbuf.String())
  // }

  // // set s.version to lib file mtime
  // libstat, err := os.Stat(libfile)
  // if err != nil {
  //   return err
  // }
  // s.version = libstat.ModTime().UnixNano()

  return nil
}*/
