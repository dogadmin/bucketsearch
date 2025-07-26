package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const baseURL = "https://buckets.grayhatwarfare.com/api/v2"

type File struct {
	ID           any    `json:"id"`
	Bucket       string `json:"bucket"`
	BucketID     any    `json:"bucketId"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	Size         int64  `json:"size"`
	Type         string `json:"type"`
	LastModified int64  `json:"lastModified"`
}

type FilesResponse struct {
	Files []File `json:"files"`
	Meta  struct {
		Results int `json:"results"`
	} `json:"meta"`
}

type Bucket struct {
	ID        any    `json:"id"`
	Bucket    string `json:"bucket"`
	FileCount int    `json:"fileCount"`
	Type      string `json:"type"`
}

type BucketsResponse struct {
	Buckets []Bucket `json:"buckets"`
	Meta    struct {
		Results int `json:"results"`
	} `json:"meta"`
}

type StatsResponse struct {
	Stats struct {
		FilesCount int64 `json:"filesCount"`
		AwsCount   int   `json:"awsCount"`
		AzureCount int   `json:"azureCount"`
		DosCount   int   `json:"dosCount"`
		GcpCount   int   `json:"gcpCount"`
		AliCount   int   `json:"aliCount"`
	} `json:"stats"`
}

func main() {
	apiKey := flag.String("apikey", os.Getenv("GHW_API_KEY"), "API key (or set env GHW_API_KEY)")
	cmd := flag.String("cmd", "files", "Command: files|buckets|stats")
	keywords := flag.String("keywords", "", "Search keywords")
	ext := flag.String("ext", "", "comma separated extensions filter, e.g. pdf,docx")
	noext := flag.String("noext", "", "comma separated extensions to exclude")
	bucket := flag.String("bucket", "", "Bucket id or url")
	limit := flag.Int("limit", 1000, "Page size (1-1000). All pages will be fetched until results exhausted")
	start := flag.Int("start", 0, "Start offset (files/buckets)")
	output := flag.String("o", "", "Output csv file path. If empty, print json")
	cloudType := flag.String("type", "", "Bucket cloud type filter: aws|azure|dos|gcp|ali")
	onlyBucket := flag.Bool("onlybucket", false, "Output only bucket names (one per line or single column CSV)")
	flag.Parse()

	if *apiKey == "" {
		log.Fatalln("missing api key")
	}

	client := &http.Client{Timeout: 15 * time.Second}

	switch strings.ToLower(*cmd) {
	case "files":
		handleFiles(client, *apiKey, *keywords, *bucket, *ext, *noext, *limit, *start, *output)
	case "buckets":
		handleBuckets(client, *apiKey, *keywords, *cloudType, *limit, *start, *output, *onlyBucket)
	case "stats":
		handleStats(client, *apiKey, *output)
	default:
		log.Fatalf("unknown cmd %s\n", *cmd)
	}
}

func buildURL(path string, params map[string]string) string {
	u, _ := url.Parse(baseURL + path)
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func doGet(client *http.Client, apiKey, urlStr string) ([]byte, error) {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func handleFiles(client *http.Client, apiKey, keywords, bucket, ext, noext string, limit, start int, output string) {
	pageSize := limit
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 1000
	}

	var allFiles []File
	var w *csv.Writer
	var fetched int64
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			log.Fatalf("create csv: %v", err)
		}
		defer f.Close()
		w = csv.NewWriter(f)
		defer w.Flush()
		w.Write([]string{"id", "bucket", "bucketId", "name", "url", "size", "type", "lastModified"})
	}

	offset := start
	total := -1
	for {
		urlStr := buildURL("/files", map[string]string{
			"keywords":       keywords,
			"bucket":         bucket,
			"extensions":     ext,
			"stopextensions": noext,
			"limit":          fmt.Sprintf("%d", pageSize),
			"start":          fmt.Sprintf("%d", offset),
		})
		data, err := doGet(client, apiKey, urlStr)
		if err != nil {
			log.Fatalf("request error: %v", err)
		}
		var resp FilesResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			log.Fatalf("decode: %v", err)
		}

		// write/collect
		if w != nil {
			for _, file := range resp.Files {
				w.Write([]string{
					fmt.Sprint(file.ID),
					file.Bucket,
					fmt.Sprint(file.BucketID),
					file.Name,
					file.URL,
					fmt.Sprintf("%d", file.Size),
					file.Type,
					time.Unix(file.LastModified, 0).Format(time.RFC3339),
				})
			}
			w.Flush()
		} else {
			allFiles = append(allFiles, resp.Files...)
		}

		atomic.AddInt64(&fetched, int64(len(resp.Files)))
		if total == -1 {
			total = resp.Meta.Results
		}
		if total > 0 {
			fmt.Printf("\r已获取 %d / %d 条", fetched, total)
		} else {
			fmt.Printf("\r已获取 %d 条", fetched)
		}

		if len(resp.Files) < pageSize || (total > 0 && offset+pageSize >= total) {
			break
		}
		offset += pageSize
	}

	fmt.Println()
	if w != nil {
		fmt.Printf("completed, saved to %s\n", output)
	} else {
		out, _ := json.MarshalIndent(allFiles, "", "  ")
		os.Stdout.Write(out)
	}
}

func handleBuckets(client *http.Client, apiKey, keywords, cloudType string, limit, start int, output string, onlyBucket bool) {
	pageSize := limit
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 1000
	}

	var allBuckets []Bucket
	var w *csv.Writer
	var fetched int64
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			log.Fatalf("create csv: %v", err)
		}
		defer f.Close()
		w = csv.NewWriter(f)
		defer w.Flush()
		if onlyBucket {
			w.Write([]string{"bucket"})
		} else {
			w.Write([]string{"id", "bucket", "fileCount", "type"})
		}
	}

	offset := start
	total := -1
	for {
		urlStr := buildURL("/buckets", map[string]string{
			"keywords": keywords,
			"type":     cloudType,
			"limit":    fmt.Sprintf("%d", pageSize),
			"start":    fmt.Sprintf("%d", offset),
		})
		data, err := doGet(client, apiKey, urlStr)
		if err != nil {
			log.Fatalf("request error: %v", err)
		}
		var resp BucketsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			log.Fatalf("decode: %v", err)
		}

		// client-side filter if cloudType specified
		filtered := resp.Buckets
		if cloudType != "" {
			var tmp []Bucket
			for _, b := range resp.Buckets {
				if strings.EqualFold(b.Type, cloudType) {
					tmp = append(tmp, b)
				}
			}
			filtered = tmp
		}

		if w != nil {
			if onlyBucket {
				for _, b := range filtered {
					w.Write([]string{b.Bucket})
				}
			} else {
				for _, b := range filtered {
					w.Write([]string{
						fmt.Sprint(b.ID),
						b.Bucket,
						fmt.Sprintf("%d", b.FileCount),
						b.Type,
					})
				}
			}
			w.Flush()
		} else {
			allBuckets = append(allBuckets, filtered...)
		}

		atomic.AddInt64(&fetched, int64(len(filtered)))
		if total == -1 {
			total = resp.Meta.Results
		}
		if total > 0 {
			fmt.Printf("\r已获取 %d / %d 条", fetched, total)
		} else {
			fmt.Printf("\r已获取 %d 条", fetched)
		}

		if len(resp.Buckets) < pageSize || (total > 0 && offset+pageSize >= total) {
			break
		}
		offset += pageSize
	}

	fmt.Println()
	if w != nil {
		fmt.Printf("completed, saved to %s\n", output)
	} else {
		if onlyBucket {
			for _, b := range allBuckets {
				fmt.Println(b.Bucket)
			}
		} else {
			out, _ := json.MarshalIndent(allBuckets, "", "  ")
			os.Stdout.Write(out)
		}
	}
}

func handleStats(client *http.Client, apiKey, output string) {
	urlStr := baseURL + "/stats"
	data, err := doGet(client, apiKey, urlStr)
	if err != nil {
		log.Fatalf("request error: %v", err)
	}
	if output == "" {
		os.Stdout.Write(data)
		return
	}
	if err := os.WriteFile(output, data, 0644); err != nil {
		log.Fatalf("write file: %v", err)
	}
	fmt.Printf("stats saved to %s\n", output)
}
