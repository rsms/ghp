package main

import (
  "bytes"
  "io"
  "io/ioutil"
  "os"
  "sync"
  "gopkg.in/yaml.v2"
)

type HttpServerConfig struct {
  Address string
  Port    uint
}

type ServletConfig struct {
  Enabled   bool
  Preload   bool
  HotReload bool `yaml:"hot-reload"`
  Recycle   bool
}

type GoConfig struct {
  Autofetch bool
  Gopath    string  // in addition to ghpdir/gopath
}

type Config struct {
  BuildDir string   `yaml:"build-dir"`
  PubDir   string   `yaml:"pub-dir"`

  HttpServer []*HttpServerConfig `yaml:"http-server"`

  DirList struct {
    Enabled  bool
    Template string
  } `yaml:"dir-list"`

  Servlet ServletConfig

  Go GoConfig

  // ---- internal ----
  goProcEnv []string
  goProcEnvOnce sync.Once
}


func (c *Config) load(r io.Reader) error {
  data, err := ioutil.ReadAll(r)
  if err != nil {
    return err
  }
  data = bytes.Replace(data, []byte("${ghpdir}"), []byte(ghpdir), -1)
  r = bytes.NewReader(data)

  d := yaml.NewDecoder(r)
  d.SetStrict(true)
  return d.Decode(c)
}


func (c *Config) writeYaml(w io.Writer) error {
  return yaml.NewEncoder(w).Encode(c)
}

// func (c *Config) writeYaml(w io.Writer) error {
//   d, err := yaml.Marshal(c)
//   if err != nil {
//     return err
//   }
//   d = bytes.Replace(d, []byte(ghpdir + "/"), []byte("${ghpdir}/"), -1)
//   d = bytes.Replace(d, []byte(ghpdir + "\n"), []byte("${ghpdir}\n"), -1)
//   d = bytes.Replace(d, []byte(ghpdir + "\""), []byte("${ghpdir}\""), -1)
//   r := bytes.NewReader(d)
//   _, err = r.WriteTo(w)
//   return err
// }



func openUserConfigFile() (*os.File, error) {
  // try different locations
  locations := []string{
    "ghp.yaml",
    "ghp.yml",
  }
  for _, name := range locations {
    f, err := os.Open(name)
    if err == nil {
      return f, nil
    }
    if !os.IsNotExist(err) {
      return nil, err
    }
  }
  return nil, nil
}


// loadConfig loads configuration from ghpdir/misc/ghp.yaml. In addition,
// this function also loads _either_ explicitFile _or_ a ghp.yaml file
// in the current directory, if one is found.
// This additional config file overrides configuration properties set by
// ghpdir/misc/ghp.yaml.
//
func loadConfig(explicitFile string) (*Config, error) {
  c := &Config{}

  // load base config
  baseConfigName := pjoin(ghpdir, "misc", "ghp.yaml")
  f, err := os.Open(baseConfigName)
  if err != nil {
    if os.IsNotExist(err) {
      err = errorf("base config file not found: %s", baseConfigName)
    }
    return nil, err
  }
  defer f.Close()
  logf("loading config %q", f.Name())
  if err = c.load(f); err != nil {
    return nil, err
  }

  // load optional user config, which can override any config properties
  if explicitFile != "" {
    f, err = os.Open(explicitFile)
  } else {
    f, err = openUserConfigFile()
  }
  if err != nil {
    return nil, err
  }
  if f != nil {
    defer f.Close()
    logf("loading config %q", f.Name())
    if err := c.load(f); err != nil {
      return nil, err
    }
  }

  // Canonicalize paths (preserves symlinks)
  c.PubDir = abspath(c.PubDir)
  c.BuildDir = abspath(c.BuildDir)
  c.DirList.Template = abspath(c.DirList.Template)
  c.Go.Gopath = abspathList(c.Go.Gopath)

  return c, nil
}
