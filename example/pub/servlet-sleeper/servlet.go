// An example of slowly responding with chunks
package main

import (
  "ghp"
  "time"
)

const sleepTime = 5

func ServeHTTP(r *ghp.Request, w ghp.Response) {
  w.Header().Set("Transfer-Encoding", "chunked")
  w.Header().Set("Content-Type", "text/html")

  w.Printf("<body>Sleeping for %d seconds...<br>\n", sleepTime)

  for i := sleepTime; i > 0; i-- {
    w.Printf("%d...<br>\n", i)
    w.Flush()
    time.Sleep(1 * time.Second)
  }

  w.Print("I'm awake! End response.</body>\n")
}
