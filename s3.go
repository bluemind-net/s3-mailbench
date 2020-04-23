package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/endpoints"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	aws_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type S3 struct {
	Endpoint     string
	Bucket       string
	Region       string
	CreateBucket bool
	Client       *aws_s3.S3
}

func NewS3(endpoint string, bucket string, region string, createbucket bool) *S3 {
	return &S3{
		Endpoint:     endpoint,
		Bucket:       bucket,
		Region:       region,
		CreateBucket: createbucket,
	}
}

func (conf *S3) UploadWorker(wg *sync.WaitGroup, jobs <-chan *string, results chan<- Result) {
	defer wg.Done()
	errcount := 0
	for messagecontent := range jobs {
		latencyTimer := time.Now()
		bytescontent := []byte(*messagecontent)
		key := fmt.Sprintf("s3bench/%x", sha1.Sum(bytescontent))
		putReq := conf.Client.PutObjectRequest(&aws_s3.PutObjectInput{
			Bucket: aws.String(conf.Bucket),
			Key:    aws.String(key),
			Body:   bytes.NewReader(bytescontent),
		})
		_, err := putReq.Send()
		if err == nil {
			results <- Result{
				Latency: time.Now().Sub(latencyTimer),
				Size:    len(*messagecontent),
			}
		} else {
			errcount++
			if errcount > 10 {
				panic(fmt.Sprintf("s3: put: too many failures. Last: %s", err))
			}
		}
	}
}

func (conf *S3) DownloadWorker(wg *sync.WaitGroup, jobs <-chan *string, results chan<- Result) {
	var err error
	defer wg.Done()
	errcount := 0
	for key := range jobs {
		if errcount > 10 {
			panic(fmt.Sprintf("s3: git: too many failures. Last: %s", err))
		}

		latencyTimer := time.Now()
		getReq := conf.Client.GetObjectRequest(&aws_s3.GetObjectInput{
			Bucket: aws.String(conf.Bucket),
			Key:    key,
		})
		resp, err := getReq.Send()
		if err != nil {
			errcount++
			continue
		}
		var buf = make([]byte, 1024*32)
		// read the s3 object body into the buffer
		size := 0
		for {
			n, err := resp.Body.Read(buf)
			size += n
			if err == io.EOF {
				break
			}
			if err != nil {
				errcount++
			}
		}
		_ = resp.Body.Close()
		results <- Result{
			Latency: time.Now().Sub(latencyTimer),
			Size:    size,
		}
	}
}

func (conf *S3) DeleteWorker(wg *sync.WaitGroup, jobs <-chan *string, results chan<- Result) {
	defer wg.Done()
	errcount := 0
	for key := range jobs {
		latencyTimer := time.Now()
		req := conf.Client.DeleteObjectRequest(&aws_s3.DeleteObjectInput{
			Bucket: aws.String(conf.Bucket),
			Key:    key,
		})
		_, err := req.Send()
		if err == nil {
			results <- Result{
				Latency: time.Now().Sub(latencyTimer),
				Size:    0,
			}
		} else {
			errcount++
			if errcount > 10 {
				panic(fmt.Sprintf("s3: put: too many failures. Last: %s", err))
			}
		}
	}
}

func (conf *S3) Setup() (err error) {
	// set the SDK region to either the one from the program arguments or else to the same region as the EC2 instance
	defaultResolver := endpoints.NewDefaultResolver()

	s3CustResolverFn := func(service, region string) (aws.Endpoint, error) {
		if service == "s3" {
			return aws.Endpoint{
				URL:           conf.Endpoint,
				SigningRegion: region,
			}, nil
		}
		return defaultResolver.ResolveEndpoint(service, conf.Region)
	}
	// gets the AWS credentials from the default file (or environment variable)
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return err
	}

	cfg.Region = conf.Region
	cfg.EndpointResolver = aws.EndpointResolverFunc(s3CustResolverFn)

	// set a 3-minute timeout for all S3 calls, including downloading the body
	cfg.HTTPClient = &http.Client{
		Timeout: time.Second * 180,
	}

	// crete the S3 client
	conf.Client = aws_s3.New(cfg)

	// custom endpoints don't generally work with the bucket in the host prefix
	if conf.Endpoint != "" {
		conf.Client.ForcePathStyle = true
	}

	if conf.CreateBucket {
		createBucketReq := conf.Client.CreateBucketRequest(&aws_s3.CreateBucketInput{
			Bucket: aws.String(conf.Bucket),
			CreateBucketConfiguration: &aws_s3.CreateBucketConfiguration{
				LocationConstraint: aws_s3.NormalizeBucketLocation(aws_s3.BucketLocationConstraint(conf.Region)),
			},
		})
		_, err = createBucketReq.Send()
		// if the error is because the bucket already exists, ignore the error
		if err != nil && !strings.Contains(err.Error(), "BucketAlreadyOwnedByYou:") {
			return err
		}
	}
	return nil
}

func (conf *S3) Test() (err error) {
	key := "s3bench/test"
	putReq := conf.Client.PutObjectRequest(&aws_s3.PutObjectInput{
		Bucket: aws.String(conf.Bucket),
		Key:    &key,
		Body:   bytes.NewReader([]byte("test")),
	})
	if _, err = putReq.Send(); err != nil {
		return err
	}
	delReq := conf.Client.DeleteObjectRequest(&aws_s3.DeleteObjectInput{
		Bucket: aws.String(conf.Bucket),
		Key:    &key,
	})
	if _, err = delReq.Send(); err != nil {
		return err
	}
	return nil
}

func (conf *S3) ListObjects(prefix string, callback func(key string) error) (err error) {
	req := conf.Client.ListObjectsRequest(&aws_s3.ListObjectsInput{
		Bucket: aws.String(conf.Bucket),
		Prefix: aws.String(prefix),
	})
	p := req.Paginate()
	for p.Next() {
		page := p.CurrentPage()
		for _, obj := range page.Contents {
			if err = callback(*obj.Key); err != nil {
				return err
			}
		}
	}
	return p.Err()
}
