package main

import (
  "bytes"
  "strings"
  "os"
  "os/exec"
  "path/filepath"
  "fmt"
)

type GoBuildError struct {
  Message string
  Details string
}

func (e *GoBuildError) Error() string {
  return e.Message
}


func makeGoBuildError(msg, abssrcdir, stderr string) *GoBuildError {
  srcdir := pubfilename(abssrcdir)

  var lines []string

  /* Example output on stderr:
  # _/Users/rsms/src/ghp/example/pub/servlet
  ./servlet.go:24:51: syntax error: unexpected newline, expecting comma or )
  */

  for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
    line = strings.TrimSpace(line)
    if len(line) == 0 {
      continue
    }
    if line[0] == '#' {
      continue
    }

    if strings.HasPrefix(line, "./") || strings.HasPrefix(line, "../") {
      p := strings.IndexByte(line, ':')
      if p != -1 {
        filename := filepath.Join(srcdir, line[0:p])
        line = filename + line[p:]
      }
    }

    lines = append(lines, line)
  }

  return &GoBuildError{
    Message: msg,
    Details: strings.Join(lines, "\n"),
  }
}


func (c *ServletCache) buildServlet(s *Servlet, libfile string) error {
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

  // set s.version to lib file mtime
  libstat, err := os.Stat(libfile)
  if err != nil {
    return err
  }
  s.version = libstat.ModTime().UnixNano()

  return nil
}
