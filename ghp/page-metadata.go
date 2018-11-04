package main

import (
  "strings"
  "gopkg.in/yaml.v2"
)


type PageMetadata struct {
  Type    string
  Parent  string
  TemplateDelims []string `yaml:"template-delims"`
  Headers map[string]string
  Custom  map[interface{}]interface{}
}


func findPageMetadataStart(source string, i int) (int, int) {
  // find start: [<SP> <TAB> <CR> <LF>]* "-"{3,} <LF>

  lines := 0

  // skip past whitespace
  i, e := 0, len(source)-1
  c := source[i]
  for c == ' ' || c == '\t' || c == '\r' || c == '\n' {
    i++
    if i == e {
      return -1, 0
    }
    if c == '\n' {
      lines++
    }
    c = source[i]
  }

  // bail if first non-whitespace char is anything but '-'
  if c != '-' {
    return -1, 0
  }

  // read dashes
  sepStart := i
  for i < e && source[i] == '-' {
    i++
  }

  // should end in newline and be 3 or more dashes
  if i == e || source[i] != '\n' || i - sepStart < 3 {
    return -1, 0
  }

  // i+1 skips the newline, lines+1 for last <LF>
  return i + 1, lines + 1
}


func findPageMetadataEnd(source string, start int) (int, int) {
  // find end: <LF> "-"{3,} (<LF> | <EOF>)

  i := start
  x := strings.Index(source[i:], "\n---")
  if x == -1 {
    return -1, -1
  }
  innerEnd := x + i
  i += x + 4

  sourceEnd := len(source) - 1

  // read any additional dashes
  for i <= sourceEnd && source[i] == '-' {
    i++
  }

  outerEnd := i

  if i <= sourceEnd { // not EOF
    // expect line break
    if source[i] != '\n' {
      return -1, -1
    }
    outerEnd += 1 // +1 skips the newline
  }

  return innerEnd, outerEnd
}


func parsePageMetadata(name string, source string) (*PageMetadata, int, error) {
  i := 0
  innerStart, nLeadingLines := findPageMetadataStart(source, i)
  if innerStart == -1 {
    return nil, 0, nil
  }

  innerEnd, outerEnd := findPageMetadataEnd(source, innerStart)
  if innerEnd == -1 {
    return nil, 0, nil
  }

  yamlSource := source[innerStart:innerEnd]

  m := &PageMetadata{}
  err := yaml.Unmarshal([]byte(yamlSource), m)

  if err != nil {
    // reparse with added blank lines to get better error message
    yamlSource = strings.Repeat("\n", nLeadingLines) + yamlSource
    err = yaml.Unmarshal([]byte(yamlSource), m)
  } else {
    // reparse as custom map
    // TODO: Find a more efficient way to parse:
    // - known fields into struct
    // - unknown fields into "Custom" map
    m.Custom = make(map[interface{}]interface{})
    yaml.Unmarshal([]byte(yamlSource), m.Custom)
  }

  return m, outerEnd, err
}
