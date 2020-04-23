package main

import (
	"errors"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/schollz/progressbar/v3"
	"io"
	"math"
	"os"
	"sync"
	"time"
	flag "github.com/spf13/pflag"
)

// CheckIfError should be used to naively panics if an error is not nil.
func CheckIfError(err error) {
	if err == nil {
		return
	}
	fmt.Printf("\x1b[31;1m%s\x1b[0m\n", fmt.Sprintf("error: %s", err))
	os.Exit(1)
}

// Info should be used to describe the example commands that are about to run.
func Info(format string, args ...interface{}) {
	fmt.Printf("\x1b[34;1m%s\x1b[0m\n", fmt.Sprintf(format, args...))
}

type Result struct {
	Latency time.Duration
	Size    int
}

var repopath string
var maxmessages uint64
var messagecount uint64 = 0

func main() {
	var endpoint string
	var bucket string
	var region string
	var createbucket bool

	var action_upload bool
	var action_download bool
	var action_clean bool
	var csvfile string
	var workers []int
	var cleanworkers int
	var csvwriter io.Writer
	var err error

	flag.StringVarP(&repopath, "public-inbox-repo", "r", "", "public-inbox repository path")
	flag.IntSliceVarP(&workers, "workers", "w", []int{4,8,16,32}, "number of workers (separated by comma)")
	flag.IntVar(&cleanworkers, "cleaning-workers", 16, "number of cleaning workers")
	flag.Uint64Var(&maxmessages, "max-messages", 100_000, "maximum messages to upload")
	flag.StringVar(&endpoint, "endpoint", "", "S3 endpoint")
	flag.StringVar(&bucket, "bucket-name", "", "S3 bucket name")
	flag.StringVar(&region, "region", "", "S3 region")
	flag.BoolVar(&createbucket, "createbucket", false, "creates the S3 bucket for you")

	flag.StringVar(&csvfile, "csv", "", "write statisics out to CSV file specified (- for Stdout)")

	flag.BoolVar(&action_upload, "upload", false, "upload test data (requires public-inbox-repo)")
	flag.BoolVar(&action_download, "download", false, "download test data (requires prior upload)")
	flag.BoolVar(&action_clean, "clean", false, "remove test data (requires prior upload)")

	flag.Parse()

	if len(repopath) == 0 && action_upload {
		fmt.Println("--public-inbox-repo is mandatory for upload")
		flag.Usage()
		os.Exit(1)
	}

	if len(bucket) == 0 {
		fmt.Println("--bucket is mandatory")
		flag.Usage()
		os.Exit(1)
	}

	if len(endpoint) == 0 {
		fmt.Println("--endpoint is mandatory")
		flag.Usage()
		os.Exit(1)
	}
	if len(region) == 0 {
		region = "fr-par"
	}
	if len(workers) == 0 {
		fmt.Println("--workers must be non empty")
		flag.Usage()
		os.Exit(1)
	}
	if len(csvfile) > 0 {
		if csvfile == "-" {
			csvwriter = os.Stdout
		} else {
			f, err := os.OpenFile(csvfile, os.O_RDWR | os.O_TRUNC | os.O_CREATE, 0644)
			defer f.Close()
			csvwriter = f
			CheckIfError(err)
		}
	}

	if ! action_upload && !action_download && !action_clean {
		fmt.Println("either --upload --download or --clean MUST be specified")
		flag.Usage()
		os.Exit(1)
	}

	statslist := make([]*Stats, 0)

	Info("s3: setup using %s", endpoint)
	s3 := NewS3(endpoint, bucket, region, createbucket)
	err = s3.Setup()
	CheckIfError(err)

	Info("s3: testing")
	err = s3.Test()
	CheckIfError(err)

	for _, workercount := range workers {
		if action_upload {
			Info("upload test with %d workers", workercount)
			stats := NewStats(fmt.Sprintf("PUT %d", workercount))
			statslist = append(statslist, stats)
			RunTest(workercount, "upload", stats, s3)
		}
		PrintStats(os.Stderr, statslist)
		if action_download {
			Info("download test with %d workers", workercount)
			stats := NewStats(fmt.Sprintf("GET %d", workercount))
			statslist = append(statslist, stats)
			RunTest(workercount, "download", stats, s3)
		}
		PrintStats(os.Stderr, statslist)
	}

	if action_clean {
		Info("clean with %d workers", cleanworkers)
		stats := NewStats(fmt.Sprintf("DEL %d", cleanworkers))
		statslist = append(statslist, stats)
		RunTest(cleanworkers, "clean", stats, s3)
	}
	PrintStats(os.Stderr, statslist)

	// CSV output
	if len(csvfile) > 0 {
		WriteCSV(csvwriter, statslist)
	}
}


func RunTest(workercount int, action string, stats *Stats, s3 *S3) {
	jobs := make(chan *string, 64)
	results := make(chan Result, 8)
	wg := &sync.WaitGroup{}

	for worker := 1; worker <= workercount; worker++ {
		if action == "upload" {
			go s3.UploadWorker(wg, jobs, results)
		} else if action == "download" {
			go s3.DownloadWorker(wg, jobs, results)
		} else if action == "clean" {
			go s3.DeleteWorker(wg, jobs, results)
		} else {
			panic("unknown action")
		}
		wg.Add(1)
	}

	max := int(math.Max(float64(maxmessages), float64(messagecount)))

	switch action {
	case "upload":
		go FeedUpload(s3, jobs, maxmessages)
	case "download":
		go FeedDownload(s3, jobs, maxmessages)
	case "clean":
		max = -1
		go FeedDownload(s3, jobs, 0)
	}

	bar := progressbar.NewOptions(
		max,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowIts(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
	)
	// read results
	go func() {
		for r := range results {
			bar.Add(1)
			stats.Update(r)
		}
	}()

	// print_stats := time.NewTicker(time.Second * 2)
	// done := make(chan bool)
	// go func() {
	// 	for {
	// 		select {
	// 		case <-done:
	// 			return
	// 		case <-print_stats.C:
	// 			if (stats.Count > 0) {
	// 				fmt.Println("")
	// 				stats.Print(os.Stdout)
	// 				fmt.Println("")
	// 			}
	// 		}
	// 	}
	// }()

	wg.Wait()
	// done <- true

	bar.Finish()
	fmt.Println("")

	// stats.Print(os.Stdout)
}

func FeedUpload(s3 *S3, jobs chan<- *string, maxmessages uint64) {
	Info("git: opening repository...")
	r, err := git.PlainOpen(repopath)
	CheckIfError(err)

	Info("git: retrieving history...")
	ref, err := r.Head()
	CheckIfError(err)
	cIter, err := r.Log(&git.LogOptions{From: ref.Hash()})
	CheckIfError(err)

	Info("git: counting objects...")
	cIter.ForEach(func(c *object.Commit) error {
		messagecount++
		if messagecount >= maxmessages {
			return errors.New("ErrStop")
		} else {
			return nil
		}
	})
	Info("git: collected %d messages", messagecount)

	var midx uint64 = 0
	cIter, _ = r.Log(&git.LogOptions{From: ref.Hash()})
	err = cIter.ForEach(func(c *object.Commit) error {
		midx++
		file, err := c.File("m")
		if err == nil {
			messagecontent, err := file.Contents()
			if err == nil {
				jobs <- &messagecontent
			}
		}
		if maxmessages != 0 && midx >= maxmessages {
			return errors.New("ErrStop")
		} else {
			return nil
		}
	})
	close(jobs)
}

func FeedDownload(s3 *S3, jobs chan<- *string, maxmessages uint64) {
	var midx uint64 = 0
	s3.ListObjects("s3bench/", func(key string) (err error) {
		jobs <- &key
		midx++
		if maxmessages != 0 && midx >= maxmessages {
			return errors.New("ErrStop")
		} else {
			return nil
		}
	})
	close(jobs)
}