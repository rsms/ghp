# Go Hypertext Preprocessor

Serve stuff over the interwebs with Go in a PHP-like fashion.

- Simply create and edit .go files in a straight-forward directory structure
- A directory with a `servlet.go` file is considered an endpoint. `func ServeHTTP(r ghp.Request, w ghp.Response)` will be called to handle HTTP requests.
- Hot-reloading at runtime without the need to restart a server.
- Source graph optionally computed live for perfect dependency knowledge — change a source file in a far-away dependency and have appropriate GHP endpoints be recompiled and reloaded.
- Dead-simple Zero-Downtime Restarts out of the box


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

Servlets can additionally provide the optional
`StartServlet` and `StopServlet` functions, called when a servlet instance
has been started and is stopping, respectively.

`StopServlet` is called just after a servlet has been disconnected from
receiving any new requests. This might happen when a new instance (version)
of the same servlet has been started, or when GHP is shutting down.

During a graceful shutdown of GHP,
for instance from SIGHUP or [ZDR](#zero-downtime-restarts),
or when a servlet instance is replaced with a newer version, `StopServlet`
may block while completing any ongoing work, like shutting down a websocket
or writing data to disk.

`StartServlet` can be useful for setting up shared resources, or for picking
up shared state from a past servlet instance.


## Zero-Downtime Restarts

GHP supports seamless restarts where the server never stops listening for
connections. ZDR is enabled by default and doesn't require you to launch
`ghp` processes in any special way.

- Coordination is per directory served. i.e. a GHP process serving a certain
  directory will coordinate with any other GHP process that is launched to
  serve the same directory.
- Works by transferring ownership of listener file descriptors via a Unix
  socket, thus ZDR works on any POSIX system where Unix sockets are enabled.
- Gracefully shuts down an older process, completeing in-flight requests while
  at the same the newer process starts serving new requests concurrently.
- Servlets can hook into this system by simply providing the optional
  `StopServlet` and `StartServlet` functions.
- Coordination can be customized using a config file by setting `zdr.group` to
  a unique string that is unique to the host machine.

Try it with the example app:

```
# Terminal 1                            Terminal 1
$ cd ghp/example && ../bin/ghp
listening on http://[::1]:8002
listening on http://localhost:8002
listening on https://localhost:8443     $ cd ghp/example && ../bin/ghp
graceful shutdown initiated             listening on http://[::1]:8002
graceful shutdown completed             listening on http://localhost:8002
$                                       listening on https://localhost:8443
$ ../bin/ghp                            graceful shutdown initiated
listening on http://[::1]:8002          graceful shutdown completed
listening on http://localhost:8002      $
listening on https://localhost:8443     $ pkill -HUP ghp
graceful shutdown initiated
graceful shutdown completed
$
```

When switching processes, try requesting
`http://localhost:8002/servlet-sleeper/` which responds very slowly piece by
piece. You'll notice that even when you start another GHP process that takes
over, an ongoing requests is patiently served til completion.


## Usage

```sh
./build.sh
(cd example && ../bin/ghp -dev)
```

Open `http://localhost:8002/`

Edit go files in `example/pub` and reload your web browser.


### Dev setup

- Terminal 1: `autorun -r=500 ghp/*.go -- ./build.sh -noget`
- Terminal 2: `(cd example && autorun ../bin/ghp -- ../bin/ghp -dev)`

Now just edit source files and GHP will be automatically rebuilt and restarted.

Get [autorun here](https://github.com/rsms/autorun)
