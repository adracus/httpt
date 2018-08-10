package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"github.com/Adracus/httpt/util"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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

type durationFlag time.Duration

func (d *durationFlag) String() string {
	return time.Duration(*d).String()
}

func (d *durationFlag) Set(value string) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	*d = durationFlag(duration)
	return nil
}

var (
	logLevel = util.LevelFlag(logrus.InfoLevel)

	method    string
	header    = headerFlag(http.Header{})
	formatter = util.FormatterFlag(util.DEFAULT)

	file string

	insecure          bool
	disableKeepAlives bool
	clientKey         string
	clientCert        string

	verbose bool

	timeout durationFlag
	wait    durationFlag

	times         int
	numWorkers    int
	monitorBuffer int

	reportStats bool
)

func init() {
	flag.Var(&logLevel, "log-level", "logging level")
	flag.Var(&formatter, "formatter", "logging formatter")

	flag.StringVar(&method, "method", http.MethodGet, "HTTP method to use for the request")
	flag.Var(&header, "H", "header value")

	flag.StringVar(&file, "f", "", "file to use for the request body")

	flag.BoolVar(&insecure, "i", false, "whether to do insecure https or not")
	flag.BoolVar(&disableKeepAlives, "disable-keep-alives", false, "whether to disable the reuse of tcp connections")
	flag.StringVar(&clientKey, "client-key", "", "path to a client key file")
	flag.StringVar(&clientCert, "client-cert", "", "path to a client cert file")

	flag.BoolVar(&verbose, "v", false, "log verbosely")

	flag.Var(&timeout, "timeout", "timeout for http requests - 0 means infinite wait")
	flag.Var(&wait, "wait", "time to wait after each http request - 0 means no wait")

	flag.IntVar(&times, "times", 1, "how many times to execute the request")
	flag.IntVar(&numWorkers, "workers", 1, "how many workers to use")
	flag.IntVar(&monitorBuffer, "monitor-buffer", 1, "how big the queue buffer to monitor should be")

	flag.BoolVar(&reportStats, "stats", false, "whether to report stats or not")
}

func needsCustomTLSConfig() bool {
	return insecure || clientKey != "" || clientCert != ""
}

func needsCustomTransport() bool {
	return disableKeepAlives || needsCustomTLSConfig()
}

func needsCustomClient() bool {
	return timeout != 0 || needsCustomTLSConfig()
}

func getTLSClientConfig() (*tls.Config, error) {
	if !needsCustomTLSConfig() {
		return nil, nil
	}

	var certificates []tls.Certificate
	if clientKey != "" && clientCert != "" {
		clientKeyData, err := ioutil.ReadFile(clientKey)
		if err != nil {
			return nil, err
		}
		clientCertData, err := ioutil.ReadFile(clientCert)
		if err != nil {
			return nil, err
		}
		keyPair, err := tls.X509KeyPair(clientCertData, clientKeyData)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, keyPair)
	}

	return &tls.Config{
		InsecureSkipVerify: insecure,
		Certificates:       certificates,
	}, nil
}

func getTransportForClient() (http.RoundTripper, error) {
	if needsCustomTransport() {
		return nil, nil
	}

	tlsConfig, err := getTLSClientConfig()
	if err != nil {
		return nil, err
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		TLSClientConfig:       tlsConfig,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     disableKeepAlives,
	}, nil
}

func getClient() (*http.Client, error) {
	if needsCustomClient() {
		return http.DefaultClient, nil
	}

	transport, err := getTransportForClient()
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeout),
	}, nil
}

type Response struct {
	Status int
	Header http.Header
	Data   []byte

	Error error
}

type Result struct {
	Start    time.Time
	Duration time.Duration

	Response *Response

	Error error
}

func doRequests(client *http.Client, req *http.Request, times int) <-chan *Result {
	out := make(chan *Result, monitorBuffer)
	go func() {
		for i := 0; i < times; i++ {
			if i > 0 {
				time.Sleep(time.Duration(wait))
			}
			out <- doRequest(client, req)
		}
		close(out)
	}()
	return out
}

func getReq(url string) (*http.Request, error) {
	var body io.ReadCloser
	if file != "" {
		var err error
		body, err = os.Open(file)
		if err != nil {
			return nil, err
		}
		defer body.Close()
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header = http.Header(header)
	return req, nil
}

func doRequest(client *http.Client, req *http.Request) *Result {
	start := time.Now()
	mkResult := func(res *Response, err error) *Result {
		return &Result{
			Start:    start,
			Duration: time.Since(start),

			Response: res,
			Error:    err,
		}
	}

	res, err := client.Do(req)
	if err != nil {
		return mkResult(nil, err)
	}
	defer res.Body.Close()

	response := &Response{
		Status: res.StatusCode,
		Header: res.Header,
	}
	response.Data, response.Error = ioutil.ReadAll(res.Body)

	return mkResult(response, nil)
}

type Worker struct {
	*logrus.Entry
	ID  int
	Out <-chan *Result
}

func NewWorker(client *http.Client, req *http.Request, id int) *Worker {
	return &Worker{
		ID:    id,
		Entry: logrus.WithField("worker", id),
		Out:   doRequests(client, req, times),
	}
}

func getWorkers(client *http.Client, req *http.Request) []*Worker {
	workers := make([]*Worker, 0, numWorkers)
	for i := 0; i < numWorkers; i++ {
		workers = append(workers, NewWorker(client, req, i))
	}
	return workers
}

func monitorWorkers(workers []*Worker) *Stats {
	var (
		wg    sync.WaitGroup
		stats Stats
	)
	wg.Add(numWorkers)

	for _, out := range workers {
		go monitorWorker(&wg, &stats, out)
	}
	wg.Wait()

	return &stats
}

type Stats struct {
	sync.Mutex
	failedRequests         []error
	failedRequestDurations []time.Duration

	failedResponseReads         []error
	failedResponseReadDurations []time.Duration

	successDurations []time.Duration
}

func (s *Stats) ReportFailedRequest(err error, duration time.Duration) {
	if reportStats {
		s.Lock()
		defer s.Unlock()

		s.failedRequests = append(s.failedRequests, err)
		s.failedRequestDurations = append(s.failedRequestDurations, duration)
	}
}

func (s *Stats) ReportFailedResponseRead(err error, duration time.Duration) {
	if reportStats {
		s.Lock()
		defer s.Unlock()

		s.failedResponseReads = append(s.failedResponseReads, err)
		s.failedResponseReadDurations = append(s.failedResponseReadDurations, duration)
	}
}

func (s *Stats) ReportSuccess(duration time.Duration) {
	if reportStats {
		s.Lock()
		defer s.Unlock()

		s.successDurations = append(s.successDurations, duration)
	}
}

func monitorWorker(wg *sync.WaitGroup, stats *Stats, worker *Worker) {
	defer wg.Done()

	for result := range worker.Out {
		worker.Debugf("duration: %s - %s", result.Start, result.Duration)

		if result.Error != nil {
			worker.Errorf("request error: %v", result.Error)
			stats.ReportFailedRequest(result.Error, result.Duration)
			continue
		}

		response := result.Response
		worker.Debugf("status: %d", response.Status)
		worker.Debugf("headers: %s", response.Header)
		if response.Error != nil {
			worker.Errorf("response read error: %v", response.Error)
			stats.ReportFailedResponseRead(response.Error, result.Duration)
			continue
		}

		worker.Print(string(response.Data))
		stats.ReportSuccess(result.Duration)
	}
}

func MeanDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return time.Duration(-1)
	}

	sum := time.Duration(0)
	for _, duration := range durations {
		sum += duration
	}
	return time.Duration(float64(sum) / float64(len(durations)))
}

func printStats(stats *Stats) {
	if reportStats {
		logrus.Infof("no of failed requests: %d", len(stats.failedRequests))
		logrus.Infof("no of failed response reads: %d", len(stats.failedResponseReads))
		logrus.Infof("no of successful requests: %d", len(stats.successDurations))

		logrus.Infof("mean time for failed requests: %s", MeanDuration(stats.failedRequestDurations))
		logrus.Infof("mean time for failed body reads: %s", MeanDuration(stats.failedResponseReadDurations))
		logrus.Infof("mean time for successful requests: %s", MeanDuration(stats.successDurations))
	}
}

func main() {
	flag.Parse()
	logrus.SetLevel(logLevel.Level())
	util.FormatterType(formatter).Apply()

	args := flag.Args()
	if len(args) != 1 {
		logrus.Fatal("Exactly one URL has to be specified as argument")
	}

	url := args[0]

	req, err := getReq(url)
	if err != nil {
		logrus.Fatalf("Could not create request: %v", err)
	}

	client, err := getClient()
	if err != nil {
		logrus.Fatalf("Could not create client: %v", err)
	}

	workers := getWorkers(client, req)

	stats := monitorWorkers(workers)

	printStats(stats)
}
