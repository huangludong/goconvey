package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/smartystreets/goconvey/web/server/parser"
	"github.com/smartystreets/goconvey/web/server/results"
)

func reactToChanges() {
	// TODO: encapsulate in a struct to reduce parameter passing (and facilitate testing?)

	busy := true
	done, ready := make(chan bool), make(chan bool)

	go runTests(done)

	for {
		select {
		case event := <-watcher.Event:
			if strings.HasSuffix(event.Name, ".go") && !busy {
				go runTests(done)
				busy = true
			}

		case err := <-watcher.Error:
			panic(err)

		case <-done:
			time.AfterFunc(100*time.Millisecond, func() {
				ready <- true
			})

		case <-ready:
			busy = false
		}
	}
}

func runTests(done chan bool) {
	// TODO: encapsulate in a struct to avoid parameter passing (and facilitate testing?)

	input, output := make(chan string), make(chan *TestPackage)
	spawnTestExecutors(input, output)
	go scheduleTestExecution(input)
	result := aggregateResults(output)
	remember(result)
	done <- true
}
func spawnTestExecutors(input chan string, output chan *TestPackage) {
	for i := 0; i < len(watched); i++ {
		go worker(input, output)
	}
}
func worker(in chan string, out chan *TestPackage) {
	for path := range in {
		out <- executeTests(path)
	}
}
func scheduleTestExecution(input chan string) {
	for folder, _ := range watched {
		input <- folder
	}
}
func aggregateResults(output chan *TestPackage) results.CompleteOutput {
	revision := md5.New()
	var packageResults []*results.PackageResult

	for _ = range watched {
		result := <-output
		io.WriteString(revision, result.Path)
		packageResults = append(packageResults, result.Parsed)
		fmt.Printf("Result for %s: [%s]\n", result.Parsed.PackageName, result.Parsed.Outcome)
	}

	return results.CompleteOutput{
		Packages: packageResults,
		Revision: hex.EncodeToString(revision.Sum(nil)),
	}
}

func executeTests(path string) *TestPackage {
	buildDependencies()
	packageName := resolvePackageName(path)
	stringOutput := testPackage(packageName)
	result := parser.ParsePackageResults(packageName, stringOutput)

	return &TestPackage{
		Path:   path,
		Output: stringOutput,
		Parsed: result,
	}
}
func buildDependencies() {
	for path, _ := range watched {
		packageName := resolvePackageName(path)
		exec.Command("go", "test", "-i", packageName).Run()
	}
}
func resolvePackageName(path string) string {
	index := strings.Index(path, "/src/")
	return path[index+len("/src/"):]
}
func testPackage(name string) string {
	fmt.Printf("Testing %s ...\n", name)
	output, _ := exec.Command("go", "test", "-v", "-timeout=-42s", name).CombinedOutput()
	return string(output)
}

func remember(output results.CompleteOutput) {
	serialized, err := json.Marshal(output)
	if err != nil {
		panic(err)
	} else {
		latestOutput = string(serialized)
	}
}

type TestPackage struct {
	Path   string
	Output string
	Parsed *results.PackageResult
}
