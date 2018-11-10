# Go Hypertext Preprocessor

Serve stuff over the interwebs with Go in a PHP-like fashion.

- Simply create and edit .go files in a straight-forward directory structure
- A directory with a `servlet.go` file is considered an endpoint. `func ServeHTTP(r ghp.Request, w ghp.Response)` will be called to handle HTTP requests.
- Hot-reloading at runtime without the need to restart a server.
- Source graph optionally computed live for perfect dependency knowledge — change a source file in a far-away dependency and have appropriate GHP endpoints be recompiled and reloaded.


### GHP page example:

`layout.ghp`:

```html
<html>
  <body>
    <h1>{.URL}</h1>
    {.Content}
  </body>
</html>
```

`page.ghp`:

```html
---
parent: parent.ghp
---
<p>Time: {timestamp}</p>
```

```
$ curl -i http://localhost:8001/page.ghp
HTTP/1.1 200 OK
Date: Sun, 04 Nov 2018 23:27:01 GMT
Content-Length: 84
Content-Type: text/html; charset=utf-8

<html>
  <body>
    <h1>/page.ghp</h1>
    <p>Time: 1541374021</p>
  </body>
</html>
```

### Servlet example

`bar/servlet.go`:

```go
package main
import "ghp"

func ServeHTTP(r ghp.Request, w ghp.Response) {
  w.WriteString("Hello world")
}
```

```
$ curl -i http://localhost:8001/bar/
HTTP/1.1 200 OK
Date: Sun, 04 Nov 2018 23:23:53 GMT
Content-Length: 11
Content-Type: text/plain; charset=utf-8

Hello world
```

## Usage

```sh
./build.sh
(cd example && ../bin/ghp -dev)
```

Open `http://localhost:8001/example/`

Edit go files in `example/pub` and reload your web browser.


### Dev setup

- Terminal 1: `autorun -r=500 ghp/*.go -- ./build.sh -noget`
- Terminal 2: `(cd example && autorun ../bin/ghp -- ../bin/ghp -dev)`

Now just edit source files and GHP will be automatically rebuilt and restarted.

Get [autorun here](https://github.com/rsms/autorun)
