package main

import (
  "bytes"
  "io"
  "io/ioutil"
  "os"
  "strings"
  "regexp"

  "gopkg.in/yaml.v2"
)

type GhpConfig struct {
  CacheDir string            `yaml:"cache-dir"`
  PubDir   string            `yaml:"pub-dir"`
  Servers  []*ServerConfig
  Zdr      ZdrConfig
  Servlet  ServletConfig
  Pages    PagesConfig
  Go       GoConfig
}

func (c *GhpConfig) onLoad() error {
  for _, sc := range c.Servers {
    if err := sc.onLoad(); err != nil {
      return err
    }
  }

  if err := c.Zdr.onLoad(); err != nil {
    return err
  }

  return nil
}


type ServerType = string
// var serverTypes = [...]string{
//   "http",
//   "https",
// }
// func (s *ServerType) UnmarshalYAML(unmarshal func(interface{}) error) error {
//   v, err := unmarshalStrEnum(serverTypes[:], defaultServerType, unmarshal)
//   *s = ServerType(v)
//   logf("server type: %v", *s)
//   return err
// }

type ServerConfig struct {
  Address     string
  Port        uint16
  Type        ServerType // http, https
  TlsCertFile string `yaml:"tls-cert-file,omitempty"`
  TlsKeyFile  string `yaml:"tls-key-file,omitempty"`
  Autocert    *AutocertConfig `yaml:",omitempty"`
  DirList     DirListConfig
}

func (c *ServerConfig) onLoad() error {
  if c.Type == "" {
    c.Type = "http"
  } else {
    c.Type = strings.ToLower(c.Type)
    if c.Type != "http" && c.Type != "https" {
      return errorf("invalid type %q in server config", c.Type)
    }
  }
  return nil
}

type AutocertConfig struct {
  // Hostnames to whitelist. (required)
  // Must be fully qualified domain names (wildcards not supported.)
  Hosts []string

  // Email optionally specifies a contact email address.
  // This is used by CAs, such as Let's Encrypt, to notify about problems
  // with issued certificates.
  Email string
}


type DirListConfig struct {
  Enabled  bool
  Template string
}


type ZdrConfig struct {
  Enabled bool
  Group   string
}

func (c *ZdrConfig) onLoad() error {
  // verify group name
  if c.Group != "" {
    groupRe := regexp.MustCompile(`^[0-9A-Za-z_\.-]+$`)
    if !groupRe.MatchString(c.Group) {
      return errorf("invalid value for zdr.group %q", c.Group)
    }
  }
  return nil
}


type ServletConfig struct {
  Enabled   bool
  Preload   bool
  HotReload bool `yaml:"hot-reload"`
  Recycle   bool
}


type PagesConfig struct {
  Enabled bool
  FileExt string `yaml:"file-ext"`
}


type GoConfig struct {
  Gopath string  // in addition to ghpdir/gopath
}

// ---------------------------------------------------------------------

func (c *GhpConfig) load(r io.Reader) error {
  data, err := ioutil.ReadAll(r)
  if err != nil {
    return err
  }
  data = bytes.Replace(data, []byte("${ghpdir}"), []byte(ghpdir), -1)
  r = bytes.NewReader(data)

  d := yaml.NewDecoder(r)
  d.SetStrict(true)

  if err := d.Decode(c); err != nil {
    return err
  }

  return c.onLoad()
}


func (c *GhpConfig) writeYaml(w io.Writer) error {
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
func loadConfig(explicitFile string) (*GhpConfig, string, error) {
  c := &GhpConfig{}

  // load base config
  filename := pjoin(ghpdir, "misc", "ghp.yaml")
  f, err := os.Open(filename)
  if err != nil {
    if os.IsNotExist(err) {
      err = errorf("base config file not found: %s", filename)
    }
    return nil, filename, err
  }
  defer f.Close()
  if devMode {
    println("loading config\n  " + f.Name())
  }
  if err = c.load(f); err != nil {
    return nil, filename, err
  }

  // load optional user config, which can override any config properties
  if explicitFile != "" {
    f, err = os.Open(explicitFile)
    filename = explicitFile
  } else {
    f, err = openUserConfigFile()
  }
  if err != nil {
    return nil, filename, err
  }

  if f != nil {
    // found user config
    defer f.Close()
    filename = abspath(f.Name())
    if devMode {
      println("  " + f.Name())
    }
    if err := c.load(f); err != nil {
      return nil, filename, err
    }
  }

  // Canonicalize paths (preserves symlinks)
  c.PubDir = abspath(c.PubDir)
  c.CacheDir = abspath(c.CacheDir)
  c.Go.Gopath = abspathList(c.Go.Gopath)

  return c, filename, nil
}
