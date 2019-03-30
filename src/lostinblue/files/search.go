package files

import (
  "bufio"
  "fmt"
  "io"
  "io/ioutil"
  "log"
  "path"
  "os"
  "regexp"
  "sort"
  "strings"
)

const MAX_RESULTS_PER_FILE = 10
const SNIPPET_LINES_ABOVE = 2
const SNIPPET_LINES_BELOW = 2

// TODO: put this in a utils file
func minInt(a, b int) int {
  if a < b {
    return a
  }
  return b
}
func maxInt(a, b int) int {
  if a > b {
    return a
  }
  return b
}

type FileData struct {
  FilePath string
  isCached bool
  cachedContents string
}

// Returns the contents of the file from cache (if cached) or by reading the file (if not cached).
// cacheResult -- stores contents in the cache (if not cached already)
func (fileData *FileData) GetContents(cacheResult bool, haltOnCacheMiss bool) (string, error) {
  if fileData.isCached {
    return fileData.cachedContents, nil
  }

  if haltOnCacheMiss {
    log.Fatal(fmt.Sprintf("halted: %v\n\n%v", fileData.FilePath, fileData.cachedContents))
  }

  byteContents, err := ioutil.ReadFile(fileData.FilePath)
  if err != nil {
    return "", err
  }
  stringContents := string(byteContents)
  if cacheResult {
    fileData.cachedContents = stringContents
    fileData.isCached = true
  }
  return stringContents, nil
}

// Dumb search (no indexing)
// - Get a list of each file within directory at any depth
// - Search each file for text
// - Return filename, line #, snippet for each match

type SearchOptions struct {
  FilePathInclude string  // Regex whitelist for paths to include search
}

type SourceLocation struct {
  Line, Col int
}
func (sl *SourceLocation) String() string {
  return fmt.Sprintf("%d:%d", sl.Line, sl.Col)
}

type FileLineInfo []int
func ParseLineStarts(contents string) (*FileLineInfo, error)  {
  contentsReader := bufio.NewReader(strings.NewReader(contents))
  charPos := 0
  line := 0
  lineStartCharIndicies := make(FileLineInfo, 0)
  for {
    lineStartCharIndicies = append(lineStartCharIndicies, charPos)
    lineContents, err := contentsReader.ReadString('\n')
    if err != nil {
      if err == io.EOF {
        break
      } else {
        return nil, err
      }
    }
    charPos += len(lineContents)  // +1 is for the newline character
    line++
  }

  return &lineStartCharIndicies, nil
}

func (fileLineInfo *FileLineInfo) GetSourceLocation(charPos int) *SourceLocation {
  nextLineIndex := sort.Search(len(*fileLineInfo), func(i int) bool { return (*fileLineInfo)[i] > charPos })
  lineStartIndex := nextLineIndex - 1
  return &SourceLocation{
    Line: lineStartIndex,
    Col: charPos - (*fileLineInfo)[lineStartIndex],
  }
}

type ResultSnippet struct{
  Start, End int
  Text string
}
type SearchFileResult struct {
  Start, End *SourceLocation
  Snippet *ResultSnippet
}
type SearchDirResults map[string][]SearchFileResult

type SearchResultsForFile struct {
  FilePath string
  Results []SearchFileResult
}

// Returns a list of filenames that are within any depth of the provided directory.
func GetFilepathsInDir(dirpath string, filePathIncludeRegexp, filePathExcludeRegexp *regexp.Regexp) ([](*FileData), error) {
  if filePathExcludeRegexp.MatchString(dirpath) {
    return nil, nil
  }

  fileInfoForFilesInDir, err := ioutil.ReadDir(dirpath)
  if err != nil {
    return nil, err
  }

  // fmt.Println("Scanning", dirpath)

  // TODO: Is this copying strings? Need to understand how Go handles ptrs, strings, arrays behind
  // the scenes

  // Builds up an array of the results from each file (can be a single file or dir) in the path
  filesInSubdirs := make([][](*FileData), len(fileInfoForFilesInDir))
  for i, fileInfo := range fileInfoForFilesInDir {
    // For now, don't follow symlinks
    if fileInfo.Mode() & os.ModeSymlink != 0 {
      continue
    }

    filePath := path.Join(dirpath, fileInfo.Name())

    // fmt.Println("Examining", filePath)

    if fileInfo.IsDir() {
      subfilesInDir, err := GetFilepathsInDir(filePath, filePathIncludeRegexp, filePathExcludeRegexp)
      if err != nil {
        return nil, err
      }
      filesInSubdirs[i] = subfilesInDir
    } else {  // Is just a file
      if filePathIncludeRegexp != nil && !filePathIncludeRegexp.MatchString(filePath) {
        // Skip file if doesn't match file path whitelist
        continue
      }
      filesInSubdirs[i] = [](*FileData){&FileData{FilePath: filePath}}
    }
  }

  // Allocate flat file array (need to figure out how many exist)
  flatFileCount := 0
  for _, filesInSubdir := range filesInSubdirs {
    if filesInSubdirs == nil {
      continue
    }
    flatFileCount += len(filesInSubdir)
  }
  flatFilesInDir := make([](*FileData), 0, flatFileCount)

  // TODO: Need to make sense of arrays vs slices, length/capacity

  // Merges the result arrays into a single array
  for _, filesInSubdir := range filesInSubdirs {
    flatFilesInDir = append(flatFilesInDir, filesInSubdir...)
  }

  return flatFilesInDir, nil
}

func SearchInFile(fileData *FileData, queryRegexp *regexp.Regexp) (*SearchResultsForFile, error) {
  var fileLineInfo *FileLineInfo

  contents, err := fileData.GetContents(false, true)
  if err != nil {
    return nil, err
  }

  // strings.Index(string(contents), query)
  allMatches := queryRegexp.FindAllStringIndex(string(contents), MAX_RESULTS_PER_FILE)
  if allMatches == nil {
    return nil, nil
  }

  fileLineInfo, err = ParseLineStarts(string(contents))
  if err != nil {
    return nil, err
  }

  fileSearchResult := make([]SearchFileResult, 0, len(allMatches))
  for _, singleMatchIndicies := range allMatches {
    // TODO: This whole section could use some refactoring - make some SearchFileResult methods
    // rather than treating it as a slice directly
    startLocation := fileLineInfo.GetSourceLocation(singleMatchIndicies[0])
    endLocation := fileLineInfo.GetSourceLocation(singleMatchIndicies[1])

    lineCount := len(*fileLineInfo)
    snippetStartLine := maxInt(startLocation.Line - SNIPPET_LINES_ABOVE, 0)
    snippetStartCharPos := (*fileLineInfo)[snippetStartLine]
    snippetEndLine := minInt(endLocation.Line + SNIPPET_LINES_BELOW + 1, lineCount - 1)
    snippetEndCharPos := (*fileLineInfo)[snippetEndLine]

    snippetText := strings.TrimRight(string(contents)[snippetStartCharPos:snippetEndCharPos], "\n")
    snippet := ResultSnippet{
      Start: singleMatchIndicies[0] - snippetStartCharPos,
      End: singleMatchIndicies[1] - snippetStartCharPos,
      Text: snippetText,
    }

    fileSearchResult = append(fileSearchResult,
      SearchFileResult{
        Start: startLocation,
        End: endLocation,
        Snippet: &snippet,
      })
  }
  return &SearchResultsForFile{FilePath: fileData.FilePath, Results:fileSearchResult}, nil
}

//const SEARCH_WORKER_COUNT = 1

func SearchWorker(
  filesToSearch chan *FileData,
  fileSearchResults chan *SearchResultsForFile,
  terminate chan bool,
  exited chan bool,
  queryRegexp *regexp.Regexp) {

  var fileToSearch *FileData = nil

  for {
    if fileToSearch != nil {
      fileSearchResult, err := SearchInFile(fileToSearch, queryRegexp)
      if err != nil {
        log.Fatal(err)
      }
      fileToSearch = nil
      if fileSearchResult != nil {
        fileSearchResults <- fileSearchResult
      }
    }
    select {
      case fileToSearch = <- filesToSearch:
        continue
      default:
    }
    select {
      case fileToSearch = <- filesToSearch:
        continue
      case <- terminate:
    }
    break
  }

  // TODO clean up code dupe
  for {
    if fileToSearch != nil {
      fileSearchResult, err := SearchInFile(fileToSearch, queryRegexp)
      if err != nil {
        log.Fatal(err)
      }
      fileToSearch = nil
      if fileSearchResult != nil {
        fileSearchResults <- fileSearchResult
      }
    }
    select {
      case fileToSearch = <- filesToSearch:
        continue
      default:
    }
    break
  }

  //fmt.Printf("Processed %v files, %v results before recv terminate\n", workerFileCount, workerResultCount)

  exited <- true
}

func PublishFilesToSearch(fileDataSet [](*FileData), fileDataChan chan *FileData, terminateWorkers chan bool, workerCount int) {
  publishedFileCount := 0

  for _, fileData := range fileDataSet {
    //fmt.Println("publish loop")
    publishedFileCount++
    fileDataChan <- fileData
  }
  //fmt.Printf("\nPublished %v files\n", publishedFileCount)

  for i := 0; i < workerCount; i++ {
    terminateWorkers <- true
  }

  //fmt.Printf("Sent terminate to workers\n")
}

func SearchInDir(fileDataSet [](*FileData), pathPrefix string, query string, options *SearchOptions, workerCount, buffering int) (SearchDirResults, error) {
  // TODO: s/terminate/exitwhendone/
  searchDirResults := make(SearchDirResults)
  filesToSearch := make(chan *FileData, buffering * workerCount)
  fileSearchResults := make(chan *SearchResultsForFile, buffering * workerCount)
  terminateWorkers := make(chan bool, workerCount)
  exitedWorkers := make(chan bool, workerCount)

  queryRegexp, err := regexp.Compile(query)
  if err != nil {
    return nil, err
  }

  go PublishFilesToSearch(fileDataSet, filesToSearch, terminateWorkers, workerCount)
  for i := 0; i < workerCount; i++ {
    go SearchWorker(filesToSearch, fileSearchResults, terminateWorkers, exitedWorkers, queryRegexp)
  }

  var fileSearchResult *SearchResultsForFile = nil
  exitedWorkerCount := 0

  for {
    if fileSearchResult != nil {
      searchDirResults[strings.TrimPrefix(fileSearchResult.FilePath, pathPrefix)] = fileSearchResult.Results
      fileSearchResult = nil
    }
    select {
      case fileSearchResult = <- fileSearchResults:
        continue
      default:
    }
    select {
      case fileSearchResult = <- fileSearchResults:
        continue
      case <- exitedWorkers:
        exitedWorkerCount++
        if exitedWorkerCount < workerCount {
          continue
        }
    }
    break
  }

  // Check if any stragglers
  // TODO clean up code dupe
  for {
    if fileSearchResult != nil {
      searchDirResults[strings.TrimPrefix(fileSearchResult.FilePath, pathPrefix)] = fileSearchResult.Results
      fileSearchResult = nil
    }
    select {
      case fileSearchResult = <- fileSearchResults:
        continue
      default:
    }
    break
  }

  return searchDirResults, nil
}
