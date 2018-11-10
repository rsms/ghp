package main

import (
  "path/filepath"
  "strings"
)

type GoBuildError struct {
  Message string
  Details string
}

func (e *GoBuildError) Error() string {
  return e.Message
}

func makeGoBuildError(msg, srcdir, stderr string) *GoBuildError {
  srcdir = pubfilename(srcdir)

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
