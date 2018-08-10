package main

import (
	"bytes"
	"flag"
	"github.com/Adracus/httpt/util"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"os"
)

var (
	logLevel = util.LevelFlag(logrus.InfoLevel)

	address string

	responsePath     string
	responseInMemory bool

	readRequest bool
)

func init() {
	flag.Var(&logLevel, "log-level", "logging level")

	flag.StringVar(&address, "address", "", "address of format <host:port> to listen on")

	flag.StringVar(&responsePath, "response", "", "path to a file containing a response")
	flag.BoolVar(&responseInMemory, "response-in-memory", false, "whether to load the response once and keep it in memory")

	flag.BoolVar(&readRequest, "read-request", false, "whether to read and print the request or not")
}

type ResponseFactory func() (io.ReadCloser, error)

func getResponseFactory() (ResponseFactory, error) {
	if responsePath == "" {
		return nil, nil
	}

	if responseInMemory {
		response, err := ioutil.ReadFile(responsePath)
		if err != nil {
			return nil, err
		}

		return func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(response)), nil
		}, nil
	}

	return func() (io.ReadCloser, error) {
		return os.Open(responsePath)
	}, nil
}

func getHandler() (http.Handler, error) {
	responseFactory, err := getResponseFactory()
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if responseFactory != nil {
			response, err := responseFactory()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				logrus.Errorf("Error reading response: %v", err)
				return
			}
			defer response.Close()

			if readRequest {
				body, err := ioutil.ReadAll(r.Body)
				if err != nil {
					logrus.Errorf("error reading request body: %v", err)
					return
				}
				logrus.Println(body)
			}

			io.Copy(w, response)
		}
	}), nil
}

func main() {
	flag.Parse()
	logrus.SetLevel(logLevel.Level())

	handler, err := getHandler()
	if err != nil {
		logrus.Fatalf("Could not create handler: %v", handler)
	}

	srv := http.Server{
		Handler: handler,
		Addr:    address,
	}

	logrus.Printf("Listening on %s (default :http)", address)
	logrus.Println(srv.ListenAndServe())
}
