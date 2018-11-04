package main

import (
  "bytes"
  "fmt"
  "html/template"
  "io/ioutil"
  "log"
  "os"
  "time"
  "sort"
)


type dirlistData struct {
  URL   string
  Name  string
  Files []os.FileInfo
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


var (
  dirlistHTMLTemplate *template.Template
  dirlistHTMLTemplateErr error
)


func loadTemplate(filename string, funcs map[string]interface{}) (*template.Template, error) {
  t := template.New(filename)
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


func getDirlistTemplate() (*template.Template, error) {
  if dirlistHTMLTemplate != nil {
    return dirlistHTMLTemplate, dirlistHTMLTemplateErr
  }
  t := template.New("dirlist")
  funcMap := make(map[string]interface{})

  funcMap["utcdate"] = helper_utcdate
  funcMap["timestamp"] = helper_timestamp
  funcMap["bytesize"] = helper_bytesize
  funcMap["fnisvisible"] = helper_fnisvisible
  funcMap["fnisroot"] = helper_fnisroot

  t, err := loadTemplate(config.DirList.Template, funcMap)
  dirlistHTMLTemplate = t
  dirlistHTMLTemplateErr = err
  if err != nil {
    log.Printf("[dirlist] failed to parse html template: %s", err.Error())
  }

  return t, err
}

type ByFilename []os.FileInfo
func (a ByFilename) Len() int           { return len(a) }
func (a ByFilename) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByFilename) Less(i, j int) bool { return a[i].Name() < a[j].Name() }

// dirlistHtml generates a HTML directory listing.
// On failure, an empty string is returned.
//
func dirlistHtml(fspath string, userpath string) ([]byte, error) {
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

  w := new(bytes.Buffer)
  t, err := getDirlistTemplate()
  if err != nil {
    return []byte{}, err
  }

  err = t.Execute(w, dirlistData{
    URL: "/no-index", // FIXME TODO
    Name: userpath,
    Files: entries,
  })

  return w.Bytes(), err
}
