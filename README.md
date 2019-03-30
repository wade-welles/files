This is a web server that exposes github.com/mjlyons/filesearch. Use it if you want
to search a directory tree's text files (say, source files) over the web.

Once running, you can query as:

http://$SERVER/search?q=$QUERY

The results will come back as JSON.

The code is pretty sloppy right now. No tests, code duplication, yikes. Please don't judge me on it, maybe one day
I'll clean this up!

## Setup example

```
> go get github.com/mjlyons/filesearch-server
> go install github.com/mjlyons/filesearch-server
> $GOBIN/filesearch-server -precache-all-files -worker-count 20 -buffering 10 -src-root $SRC_PATH
```

Note, don't use a '/' at the end of $SRC_PATH.

MIT License.

