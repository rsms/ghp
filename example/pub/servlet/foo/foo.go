package foo

import (
  "../bar"
  "./lol"
  // "./my-new-pkg"
  // "ghp"
)

func Foo() string {
  return "foo/" + bar.Bar() + "/" + lol.Lol()
}
