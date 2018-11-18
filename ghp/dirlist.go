package main

import (
  "bytes"
  "fmt"
  "html/template"
  "io/ioutil"
  "sync"
  "os"
  "time"
  "sort"
)


type HtmlDirLister struct {
  pubdir string
  t      *template.Template
}


func NewHtmlDirLister(pubdir string, c *DirListConfig) (*HtmlDirLister, error) {
  d := &HtmlDirLister{ pubdir: pubdir }

  if c.Template == "" {
    d.t = d.loadDefaultTemplate()
  } else {
    t, err := d.loadTemplate(c.Template)
    if err != nil {
      return d, err
    }
    d.t = t
  }

  return d, nil
}


var (
  defaultTemplateOnce sync.Once
  defaultTemplate *template.Template
)

func (d *HtmlDirLister) loadDefaultTemplate() *template.Template {
  defaultTemplateOnce.Do(func() {
    t, err := d.loadTemplate(pjoin(ghpdir, "misc", "dirlist.html"))
    if err != nil {
      panic(err)
    }
    defaultTemplate = t
  })
  return defaultTemplate
}


func (d *HtmlDirLister) loadTemplate(filename string) (*template.Template, error) {
  funcs := make(map[string]interface{})

  funcs["utcdate"] = helper_utcdate
  funcs["timestamp"] = helper_timestamp
  funcs["bytesize"] = helper_bytesize
  funcs["fnisvisible"] = helper_fnisvisible
  funcs["fnisroot"] = helper_fnisroot

  // load template
  t := template.New(relfile(d.pubdir, filename))
  t.Funcs(funcs)
  t.Option("missingkey=zero")
    // [missingkey=zero]
    //   The operation returns the zero value for the map type's element
  data, err := ioutil.ReadFile(filename)
  if err == nil {
    _, err = t.Parse(string(data))
  }
  return t, err
}


type dirlistData struct {
  Name  string
  Files []os.FileInfo
}

type ByFilename []os.FileInfo
func (a ByFilename) Len() int           { return len(a) }
func (a ByFilename) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByFilename) Less(i, j int) bool { return a[i].Name() < a[j].Name() }


// RenderHtml generates a HTML directory listing.
//
func (d *HtmlDirLister) RenderHtml(fspath string, userpath string) ([]byte, error) {
  file, err := os.Open(fspath)
  if err != nil {
    return []byte{}, err
  }
  defer file.Close()

  entries, err := file.Readdir(0)
  if err != nil {
    return []byte{}, err
  }

  sort.Sort(ByFilename(entries))

  var w bytes.Buffer
  err = d.t.Execute(&w, dirlistData{
    Name: userpath,
    Files: entries,
  })

  return w.Bytes(), err
}


const utcTimeFormat = "2006-01-02 15:04:05 UTC"


func helper_utcdate(v interface{}) string {
  if t, ok := v.(time.Time); ok {
    return t.UTC().Format(utcTimeFormat)
  }
  return "!{utcdate}"
}

func helper_timestamp(v interface{}) int64 {
  if t, ok := v.(time.Time); ok {
    return t.UTC().Unix()
  }
  return 0
}

func helper_bytesize(z int64) string {
  if z < 1000 {
    return fmt.Sprintf("%d B", z)
  }
  if z < 1024 * 1024 {
    return fmt.Sprintf("%d kB", z / 1024)
  }
  return fmt.Sprintf("%0.1f MB", float64(z) / (1024.0 * 1024.0))
}

func helper_fnisvisible(s string) bool {
  return len(s) > 0 && s[0] != '.'
}

func helper_fnisroot(s string) bool {
  return s == "/"
}

