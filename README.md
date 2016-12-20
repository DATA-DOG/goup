# Goup

Watches and restarts **go** applications on source code changes.

<p align="center"><img
src="https://cloud.githubusercontent.com/assets/132389/21023210/4067787a-bd88-11e6-8f0f-bffb4434f5cc.png"
alt="Goup screenshot" /></p>

    go get github.com/DATA-DOG/goup

Main features:
- just run **goup** where your main package is.
- forwards **stdin** and all command line arguments.
- **goup** itself has no options.
- does not use **go build** but rather **go install** which increases
  restart performance dramatically.
- can watch applications outside **$GOPATH** run it like
  `GOBIN=$GOPATH/bin goup -my-cmd-option args < stdin` or export
  **$GOBIN**
- tracks all dependent non **$GOROOT** packages, including all in
  **$GOPATH**, except in `vendor` directories.
- restarts only if **go install** was successful, so you do not have
  failed state.
- is about **200 loc** simple to copy and adapt to your custom needs.

