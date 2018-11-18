package main

import (
  "ghp"
  "time"
  "fmt"
)

func ServeHTTP(r *ghp.Request, w ghp.Response) {
  w.Header().Set("Transfer-Encoding", "chunked")
  w.Header().Set("X-Content-Type-Options", "nosniff")

  w.WriteString("Sleeping for 5 seconds...\n")

  for i := 5; i > 0; i-- {
    fmt.Fprintf(w, "%d...\n", i)
    w.Flush()
    time.Sleep(1 * time.Second)
  }

  w.WriteString("I'm awake! End response.\n")
}
