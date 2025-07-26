# bucketsearch

基于 buckets.grayhatwarfare.com 官方api文档写的一个 搜索工具

Usage of ./main:
  -apikey string
    	API key (or set env GHW_API_KEY)
  -bucket string
    	Bucket id or url
  -cmd string
    	Command: files|buckets|stats (default "files")
  -ext string
    	comma separated extensions filter, e.g. pdf,docx
  -keywords string
    	Search keywords
  -limit int
    	Page size (1-1000). All pages will be fetched until results exhausted (default 1000)
  -noext string
    	comma separated extensions to exclude
  -o string
    	Output csv file path. If empty, print json
  -onlybucket
    	Output only bucket names (one per line or single column CSV)
  -start int
    	Start offset (files/buckets)
  -type string
    	Bucket cloud type filter: aws|azure|dos|gcp|ali


自己编译
