# Go Hypertext Preprocessor

Serve stuff over the interwebs with Go in a PHP-like fashion.

- Simply create and edit .go files in a straight-forward directory structure
- A directory with a `servlet.go` file is considered an endpoint. `func ServeHTTP(r ghp.Request, w ghp.Response)` will be called to handle HTTP requests.
- Hot-reloading at runtime without the need to restart a server.
- Source graph optionally computed live for perfect dependency knowledge — change a source file in a far-away dependency and have appropriate GHP endpoints be recompiled and reloaded.

Example:

`hello/index.go`

```go
func ServeHTTP(r ghp.Request, w ghp.Response) {
  w.WriteString("Hello world")
}
```

## Usage

```
./init.sh  # just needed first time
./build.sh
(cd example && ../build/ghp)
```

Open `http://localhost:8001/example/`

Edit go files in `example/pub` and reload your web browser.


### Dev setup:

- Terminal 1: `autorun -r=500 ghp/*.go -- ./build.sh`
- Terminal 2: `cd example && autorun ../build/ghp -- ../build/ghp`

Get [autorun here](https://github.com/rsms/autorun)
