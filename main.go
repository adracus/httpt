package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

type headerFlag http.Header

func (s *headerFlag) String() string {
	var buf bytes.Buffer
	http.Header(*s).Write(&buf)
	return buf.String()
}

func (s *headerFlag) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	var k, v string
	if len(parts) == 1 {
		k = value
		v = value
	} else {
		k = parts[0]
		v = parts[1]
	}

	http.Header(*s).Add(k, v)
	return nil
}

var (
	method string
	header = headerFlag(http.Header{})

	file string

	dumpRequest  bool
	dumpResponse bool

	dumpResponseBody bool
)

func init() {
	flag.Var(&header, "H", "header value")
	flag.StringVar(&method, "method", http.MethodGet, "HTTP method to use for the request")
	flag.StringVar(&file, "f", "", "file to use for the request body")
	flag.BoolVar(&dumpRequest, "dump-request", false, "Whether to dump the request or not")
	flag.BoolVar(&dumpResponse, "dump-response", false, "Whether to dump the response or not")
	flag.BoolVar(&dumpResponseBody, "dump-response-body", false, "Whether to dump the response body or not")
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		panic("Exactly one URL has to be specified as argument")
	}

	url := args[0]

	var body io.ReadCloser
	if file != "" {
		var err error
		body, err = os.Open(file)
		if err != nil {
			panicf("Could not open file %q: %v", err)
		}
		defer body.Close()
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		panicf("Could not create request: %v", err)
	}
	req.Header = http.Header(header)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panicf("Could not execute request: %v", err)
	}
	defer res.Body.Close()

	if dumpRequest {
		dumpedRequest, err := httputil.DumpRequest(res.Request, false)
		if err != nil {
			fmt.Printf("Could not dump request: %v\n", err)
		}
		fmt.Printf("Request:\n%s\n", dumpedRequest)
	}

	if dumpResponse {
		dumpedResponse, err := httputil.DumpResponse(res, dumpResponseBody)
		if err != nil {
			fmt.Printf("Could not dump response: %v", err)
		}
		fmt.Printf("Response:\n%s\n", dumpedResponse)
	}

	if !dumpResponseBody {
		io.Copy(os.Stdout, res.Body)
	}
}
