package main

import (
  "strings"
  "io"
  "net/http"
  template "html/template"
)


type Page struct {
  srcpath  string
  name     string
  mtime    int64  // UnixNano
  fileid   uint64 // source file identifier e.g. inode
  builderr error  // non-nil when building failed
  relatedPageMissing string // non-empty when a related page is missing
  t        Template
  meta     *PageMetadata  // nil if there's no metadata
  parent   *Page          // nil when none
}


// alt name ideas for "wrapper page":
// - wrapper
// - parent
// - base
// - layout
//


type pageData struct {
  URL       string
  Subtitle  string
  Meta      *PageMetadata
  Content   template.HTML
}


// Serve serves the page as response w for request r
//
func (p *Page) Serve(w http.ResponseWriter, r *http.Request) error {
  if p.meta != nil && len(p.meta.Headers) > 0 {
    header := w.Header()
    for name, value := range p.meta.Headers {
      header.Add(name, value)
    }
  }
  return p.Render(w, r)
}


// Render outputs the page to w for request r
//
func (p *Page) Render(w io.Writer, r *http.Request) error {
  d := &pageData{
    URL: r.URL.Path,
    Subtitle: "subtitle here",
    Meta: p.meta,
  }
  if p.parent != nil {
    return p.renderWithParent(w, d)
  } else {
    return p.t.Exec(w, d)
  }
}


// renderSimple executes the root template.
// Used internally by Render.
//
func (p *Page) renderSimple(w io.Writer, r *http.Request, d *pageData) error {
  return p.t.Exec(w, d)
}


// renderWithParent executes all parent templates in order.
// Used internally by Render.
//
func (p *Page) renderWithParent(w io.Writer, d *pageData) error {
  // Note on seen: This is disabled as we do the cycle check during building.
  // Kept here commented-out until we have done enough testing to be certain
  // we don't need it.
  // seen := make(map[*Page]bool, 10)

  var content string
  var err error

  page := p

  for page.parent != nil {
    d.Content = template.HTML(content)
    content, err = p.renderTemplateString(page.name, d)
    if err != nil {
      return err
    }
    page = page.parent

    // if _, ok := seen[page]; ok {
    //   return errorf("cyclic templates(1): %v ... %v", p.name, page.name)
    // }
    // seen[page] = true
  }

  // if _, ok := seen[page]; ok {
  //   return errorf("cyclic templates(2): %v ... %v", p.name, page.name)
  // }
  // seen[page] = true

  d.Content = template.HTML(content)
  return p.renderTemplate(w, page.name, d)
}


// renderTemplate executes a specific named template
//
func (p *Page) renderTemplate(w io.Writer, templateName string, d *pageData) error {
  return p.t.ExecNamed(w, templateName, d)
}


// renderTemplateString executes a specific named template and returns
// the result as a string.
//
func (p *Page) renderTemplateString(templateName string, d *pageData) (string, error) {
  sb := strings.Builder{}
  err := p.renderTemplate(&sb, templateName, d)
  if err != nil {
    return "", err
  }
  return sb.String(), nil
}


// // ModTime returns the maximum modification time of the page and any parents.
// //
// func (p *Page) ModTime() int64 {
//   mtime := p.mtime
//   for p.parent != nil {
//     p = p.parent
//     if p.mtime > mtime {
//       mtime = p.mtime
//     }
//   }
//   return mtime
// }

