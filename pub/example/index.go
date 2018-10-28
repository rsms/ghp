package main

import (
  "ghp"
  "./foo"
  "./bar"
)

func ServeHTTP(r ghp.Request, w ghp.Response) {
  w.WriteString("Hello from example plugin.\n")
  w.WriteString("foo.Foo() => " + foo.Foo() + "\n")
  w.WriteString("bar.Bar() => " + bar.Bar() + "\n")
}

func main() {
  print("main")
}
