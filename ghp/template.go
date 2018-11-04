package main

import (
  "io"
  html_template "html/template"
  text_template "text/template"
  tparse "text/template/parse"
)


type Template interface {
  Name() string
  AddParseTree(name string, tree *tparse.Tree) error
  AddParseTree1(name string, tree *tparse.Tree) (Template, error)
  Option(option string)
  Funcs(funcs map[string]interface{})
  Exec(w io.Writer, data interface{}) error
  ExecNamed(w io.Writer, name string, data interface{}) error
  Templates() []Template
  Tree() *tparse.Tree
}


func NewHtmlTemplate(name string) Template {
  return &htmlTemplate{html_template.New(name)}
}

func NewTextTemplate(name string) Template {
  return &textTemplate{text_template.New(name)}
}

// ------------------------------------------------------------------------

type htmlTemplate struct {
  t *html_template.Template
}

func (t *htmlTemplate) AddParseTree(name string, tree *tparse.Tree) error {
  _, err := t.t.AddParseTree(name, tree)
  return err
}

func (t *htmlTemplate) AddParseTree1(name string, tree *tparse.Tree) (Template, error) {
  t2, err := t.t.AddParseTree(name, tree)
  if err != nil {
    return nil, err
  }
  return &htmlTemplate{t2}, err
}

func (t *htmlTemplate) Name() string { return t.t.Name() }
func (t *htmlTemplate) Option(option string) { t.t.Option(option) }
func (t *htmlTemplate) Funcs(funcs map[string]interface{}) { t.t.Funcs(funcs) }
func (t *htmlTemplate) Exec(w io.Writer, data interface{}) error { return t.t.Execute(w, data) }
func (t *htmlTemplate) ExecNamed(w io.Writer, name string, data interface{}) error { return t.t.ExecuteTemplate(w, name, data) }
func (t *htmlTemplate) Tree() *tparse.Tree { return t.t.Tree }

func (t *htmlTemplate) Templates() []Template {
  src := t.t.Templates()
  tv := make([]Template, len(src))
  for i, t2 := range src {
    tv[i] = &htmlTemplate{t2}
  }
  return tv
}

// ------------------------------------------------------------------------

type textTemplate struct {
  t *text_template.Template
}

func (t *textTemplate) AddParseTree(name string, tree *tparse.Tree) error {
  _, err := t.t.AddParseTree(name, tree)
  return err
}

func (t *textTemplate) AddParseTree1(name string, tree *tparse.Tree) (Template, error) {
  t2, err := t.t.AddParseTree(name, tree)
  if err != nil {
    return nil, err
  }
  return &textTemplate{t2}, err
}

func (t *textTemplate) Name() string { return t.t.Name() }
func (t *textTemplate) Option(option string) { t.t.Option(option) }
func (t *textTemplate) Funcs(funcs map[string]interface{}) { t.t.Funcs(funcs) }
func (t *textTemplate) Exec(w io.Writer, data interface{}) error { return t.t.Execute(w, data) }
func (t *textTemplate) ExecNamed(w io.Writer, name string, data interface{}) error { return t.t.ExecuteTemplate(w, name, data) }
func (t *textTemplate) Tree() *tparse.Tree { return t.t.Tree }

func (t *textTemplate) Templates() []Template {
  src := t.t.Templates()
  tv := make([]Template, len(src))
  for i, t2 := range src {
    tv[i] = &textTemplate{t2}
  }
  return tv
}
