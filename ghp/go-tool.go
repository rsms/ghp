package main

import (
  "bytes"
  "os"
  "os/exec"
  "path/filepath"
  "runtime"
  "strings"
  "regexp"
)

type GoTool struct {
  *exec.Cmd
}


var (
  goToolGoroot   string
  goToolFilename string
  goToolEnv      []string
)


func NewGoTool(arg... string) *GoTool {
  g := &GoTool{
    Cmd: &exec.Cmd{
      Path: goToolFilename,
      Args: append([]string{goToolFilename}, arg...),
    },
  }
  g.Cmd.Env = goToolEnv
  return g
}


func (g *GoTool) RunBufferedIO() (stdout bytes.Buffer, stderr bytes.Buffer, err error) {
  g.Cmd.Stdout = &stdout
  g.Cmd.Stderr = &stderr
  err = g.Cmd.Run()
  return
}


// ------------------------------------------------------------------------
// gotool initialization


// InitGoTool initializes the go tool.
// Config must be loaded when this function is called.
//
func InitGoTool(c *GoConfig) error {
  goToolGoroot = pjoin(ghpdir, "go")
  goToolFilename = pjoin(goToolGoroot, "bin", "go")
  
  runtimeVersion := runtime.Version() + "." + runtime.GOOS + "-" + runtime.GOARCH

  // assume the executable already exists
  st, err := os.Stat(goToolFilename)
  if err != nil {
    if !os.IsNotExist(err) {
      return errorf("go tool %q unreadable: %s", goToolFilename, err.Error())
    }

    return errorf("go not found at %q", goToolGoroot, runtimeVersion)
  }

  // make sure it's an executable file
  mode := st.Mode()
  if !mode.IsRegular() {
    return errorf("%q is not a file", goToolFilename)
  }
  if mode.Perm() & 0111 == 0 {
    // Not executable. Attempt to repair permissions.
    if err = os.Chmod(goToolFilename, 0777); err != nil {
      return errorf(
        "%q is not an executable file and chmod failed with %s",
        goToolFilename,
        err.Error(),
      )
    }
  }

  // make sure the version available is compatible with the runtime we
  // are currently running.
  installedVersion, err := getGoProgramVersion(goToolFilename)
  if err != nil {
    return err
  }
  if installedVersion != runtimeVersion {
    return errorf(
      "%s installed at %q is different from %s used to build GHP.",
      installedVersion,
      goToolGoroot,
      runtimeVersion,
    )
  }

  initGoToolEnv(c)

  if devMode {
    logf("using go tool %q", goToolFilename)
  }

  return nil
}


// Returns e.g. "go1.11.2.darwin-amd64"
func getGoProgramVersion(program string) (string, error) {
  // check version of program by executing "go version"
  cmd := exec.Command(program, "version")
  cmd.Env = append(os.Environ(),
    "GOROOT=" + filepath.Dir(filepath.Dir(program)),
  )
  var outbuf bytes.Buffer
  cmd.Stdout = &outbuf
  if err := cmd.Run(); err != nil {
    return "", err
  }
  stdout := outbuf.String()

  // Parse output.
  // It looks like this: "go version go1.11.2 darwin/amd64"
  re := regexp.MustCompile(`(?:^|\s+)(go\d+\.\d+\.\d+)\s+([^/]+)/(.+)`)
  sv := re.FindStringSubmatch(stdout)
  if len(sv) != 4 {
    return "", errorf("'go version' returned unparsable output %q", stdout)
  }

  version := sv[1] + "." + sv[2] + "-" + sv[3]
  return version, nil
}


func parseEnvEntries(entries []string) map[string]string {
  // parse os.Environ into map
  env := make(map[string]string)
  for _, entry := range entries {
    i := strings.IndexByte(entry, '=')
    if i == -1 {
      env[entry] = ""
    } else {
      env[entry[:i]] = entry[i+1:]
    }
  }
  return env
}


func initGoToolEnv(c *GoConfig) {
  listSep := string(filepath.ListSeparator)

  // parse os.Environ into map
  env := parseEnvEntries(os.Environ())

  // extend or add GOPATH
  if gopath, ok := env["GOPATH"]; ok {
    env["GOPATH"] = c.Gopath + listSep + gopath
  } else {
    env["GOPATH"] = c.Gopath
  }

  env["GOROOT"] = goToolGoroot
  // env["CGO_ENABLED"] = "0"

  // encode as K=V
  goToolEnv = []string{}
  for k, v := range env {
    goToolEnv = append(goToolEnv, k + "=" + v)
  }
}

