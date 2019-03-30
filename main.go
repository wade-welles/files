package main

import (
  "time"
  "fmt"
  "log"
  "regexp"
  "net/http"
  "encoding/json"

  "lostinblue/files"
  flags "gopkg.in/alecthomas/kingpin.v2"
)

const PATH_WHITELIST string = "\\.(py|js|coffee|go|yaml|scss|css|html|c|cpp|m|h|java)$"
const PATH_BLACKLIST string = "/(node_modules|build|coverage)/"

var searchableFiles [](*files.FileData)

// TODO: Add functionality to automatically reindex changes, perhaps track changes
var (
  path = flags.Flag("path", "Path to index with bevel/boltdb and expose a webUI for searching").Default(".").String()
  port = flags.Flag("port", "A port number to serve the WebUI").Default("8989").String()
  workers = flags.Flag("workers", "Number of workers to use during the chaching process").Default("10").Int()
  buffering = flags.Flag("buffering", "?").Default("10").Int()
)

// TODO: Cache the files in a BoltDB database, with an option for persistence vs ephemeral
// Loads each file's contents into the in-memory cache
func cacheAllFiles(fileDataSet [](*files.FileData)) error {
  for _, fileData := range fileDataSet {
    _, err := fileData.GetContents(true, false)
    if err != nil {
      return err
    }
  }
  return nil
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
  query := r.URL.Query().Get("q")
  if query == "" {
    fmt.Fprintf(w, "Ya need a ?q=")
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  fmt.Printf("Searching for %s...", query)
  startTime := time.Now()

  searchOptions := files.SearchOptions{FilePathInclude: ""}
  searchResults, err := files.SearchInDir(searchableFiles, *path, query, &searchOptions, *workers, *buffering)

  if err != nil {
    log.Fatal(err)
  }

  w.Header().Set("Content-Type", "application/json; charset=UTF-8")

  searchTime := time.Since(startTime)
  json.NewEncoder(w).Encode(searchResults)

  //msecDelay := time.Since(startTime) / time.Millisecond
  totalTime := time.Since(startTime)
  fmt.Printf("Found %v files (%v latency, %v total)\n", len(searchResults), searchTime, totalTime)
}

func main() {
  fmt.Println("Building file list...")
  startupStartTime := time.Now()
  flags.Parse()

  fs := files.New(files.Options{})

  // File path context, filtering
  // TODO: Add better, more united, easily configurable (*.yaml)
  // TODO: Globbing for the path to determine this, for example: ../*.yaml
  var filePathIncludeRegexp, filePathExcludeRegexp *regexp.Regexp
  var err error
  filePathIncludeRegexp, err = regexp.Compile(PATH_WHITELIST)
  if err != nil {
    log.Fatal(err)
  }
  filePathExcludeRegexp, err = regexp.Compile(PATH_BLACKLIST)
  if err != nil {
    log.Fatal(err)
  }
  searchableFiles, err = files.GetFilepathsInDir(*path, filePathIncludeRegexp, filePathExcludeRegexp)
  if err != nil {
    log.Fatal(err)
  }

  // You always precache, I find it hard to think of a sitaution
  // where you would not want to precache, removing this option makes
  // the software less itimindating
  // TODO: Update the cache on file change, determine best file watcher
  fmt.Println("Caching file contents...")
  err = cacheAllFiles(searchableFiles)
  if err != nil {
    log.Fatal(err)
  }
  startupDuration := time.Since(startupStartTime)
  log.Printf("Listening... (startup took %v)\n", startupDuration)

  // HTTP router
  // TODO: Switch out with gin
  http.Handle("/", fs.Serve(http.Dir(*path)))
  http.HandleFunc("/search", handleQuery)

  fmt.Println("Serving the webUI on: ", *port)

  // TODO: Default shell, allow searching from the shell or changing folders or adding new globs.
  if err := http.ListenAndServe((":" + *port), nil); err != nil {
    log.Fatal("ListenAndServe: ", err)
  }
}

