package main

import (
  "strconv"
)

// servletContext is the implementation of ghp.ServletContext
type servletContext struct {
  s *Servlet
}

func (c *servletContext) Version() string {
  return strconv.FormatInt(c.s.version, 16)
}

func (c *servletContext) Name() string {
  return c.s.name
}
