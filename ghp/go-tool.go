package main

import (
  "bytes"
  "compress/gzip"
  "net/http"
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
//
// It might download it from the internet in case Config.Go.Autofetch is
// enabled. This means it could take some time.
//
// Config must be loaded when this function is called.
//
func InitGoTool() error {
  goToolGoroot = pjoin(ghpdir, "go")
  goToolFilename = pjoin(goToolGoroot, "bin", "go")
  
  runtimeVersion := runtime.Version() + "." + runtime.GOOS + "-" + runtime.GOARCH

  // assume the executable already exists
  st, err := os.Stat(goToolFilename)
  if err != nil {
    if !os.IsNotExist(err) {
      return errorf(
        "go tool is unreadable (%s). Check %q",
        err.Error(),
        goToolFilename,
      )
    }

    return errorf(
      "go not found at %q. Get it from https://dl.google.com/go/%s.tar.gz",
      goToolGoroot,
      runtimeVersion,
    )

    // TODO: copy from system if available
    //
    // // Stat again
    // st, err = os.Stat(goToolFilename)
    // if err != nil {
    //   return err
    // }
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

  initGoToolEnv()

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


func fetchGoProgram(name, filename string) error {
  // Does the system contain the required go version?
  sysfile, err := findSystemGoProgram()
  if sysfile != "" {
    if err == nil {
      _, err = copyfile(sysfile, filename)
    }
    return err
  }
  
  // We did not find the go program locally.

  // Log any error from findSystemGoProgram (ignored)
  if err != nil && devMode {
    logf("findSystemGoProgram: %s", err.Error())
  }

  if config.Go.Autofetch {
    // Autofetch is enabled -- fetch the go binary from the internet
    return fetchGoProgramRemote(name, filename)
  }

  return errorf(
    "go binary %q not found "+
    "(enable go.autofetch in config for automatic download)",
    filename,
  )
}


func findSystemGoProgram() (string, error) {
  gofile := pjoin(runtime.GOROOT(), "bin", "go")

  st, err := os.Stat(gofile)
  if os.IsNotExist(err) || !st.Mode().IsRegular() {
    return "", nil
  }

  // check version of program by executing "go version"
  cmd := exec.Command(gofile, "version")
  cmd.Env = append(os.Environ(),
    "GOROOT=" + runtime.GOROOT(),
  )
  var outbuf bytes.Buffer
  cmd.Stdout = &outbuf
  if err = cmd.Run(); err != nil {
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
  if sv[1] != runtime.Version() {
    return "", errorf("version mismatch %q != %q", sv[1], runtime.Version())
  }
  if sv[2] != runtime.GOOS {
    return "", errorf("platform mismatch %q != %q", sv[2], runtime.GOOS)
  }
  if sv[3] != runtime.GOARCH {
    return "", errorf("arch mismatch %q != %q", sv[3], runtime.GOARCH)
  }

  // version, platform and arch matches
  return gofile, nil
}


func fetchGoProgramRemote(name, filename string) error {
  archiveFile := name + ".tar.gz"
  url := "https://dl.google.com/go/" + archiveFile
  logf("fetching %v", url)

  // HTTP GET
  res, err := http.Get(url)
  if err != nil {
    return err
  }
  defer res.Body.Close()

  // write to temporary file
  tmpfile := pjoin(os.TempDir(), filepath.Base(filename))

  // gzip filter
  gzf, err := gzip.NewReader(res.Body)
  if err != nil {
    panic(err)
  }

  // extract bin/go
  if err = TarExtractFile(gzf, "go/bin/go", tmpfile); err != nil {
    os.Remove(tmpfile)
    return err
  }

  if err = os.MkdirAll(filepath.Dir(filename), 0777); err != nil {
    os.Remove(tmpfile)
    return err
  }

  return os.Rename(tmpfile, filename)
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


func initGoToolEnv() {
  listSep := string(filepath.ListSeparator)

  // parse os.Environ into map
  env := parseEnvEntries(os.Environ())

  // extend or add GOPATH
  if gopath, ok := env["GOPATH"]; ok {
    env["GOPATH"] = config.Go.Gopath + listSep + gopath
  } else {
    env["GOPATH"] = config.Go.Gopath
  }

  env["GOROOT"] = goToolGoroot

  // encode as K=V
  goToolEnv = []string{}
  for k, v := range env {
    goToolEnv = append(goToolEnv, k + "=" + v)
  }
}

