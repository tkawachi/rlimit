package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"sync/atomic"
	"time"

	"github.com/tkawachi/rlimit"
	"gopkg.in/urfave/cli.v1"
)

type LimitedRoundTripper struct {
	ch         chan int
	inner      http.RoundTripper
	nWaiting   uint32
	maxWaiting uint32
	rate       rlimit.Rate
}

func NewLimitedRoundTripper(inner http.RoundTripper, rate rlimit.Rate, maxWaiting uint32) *LimitedRoundTripper {
	return &LimitedRoundTripper{
		ch:         make(chan int, rate.Count),
		inner:      inner,
		nWaiting:   0,
		maxWaiting: maxWaiting,
		rate:       rate,
	}
}

func (lrt *LimitedRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {

	waiting := atomic.LoadUint32(&lrt.nWaiting)
	if waiting > lrt.maxWaiting {
		resp = &http.Response{
			// Request: req,
			StatusCode: 429,
			Status:     "Too Many Requests",
			Body:       ioutil.NopCloser(bytes.NewBufferString("Too Many Requests (rlimit)\n")),
		}
		return
	}
	someInt := 0

	atomic.AddUint32(&lrt.nWaiting, 1)
	lrt.ch <- someInt
	atomic.AddUint32(&lrt.nWaiting, ^uint32(0)) // decrement
	defer func() {
		go func() {
			time.Sleep(lrt.rate.Duration)
			<-lrt.ch
		}()
	}()
	return lrt.inner.RoundTrip(req)
}

func main() {
	app := cli.NewApp()
	app.Name = "rlimit"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "forward",
			Usage: "Forward `URL`",
		},
	}

	app.Action = action
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
func action(c *cli.Context) (err error) {
	forwardURL, err := rlimit.ParseURL(c.String("forward"))
	if err != nil {
		log.Println("Specify a forward URL with --forward.")
		return
	}

	director := func(request *http.Request) {
		url := *request.URL
		url.Scheme = forwardURL.Scheme
		url.Host = forwardURL.Host

		req, err := http.NewRequest(request.Method, url.String(), request.Body)
		if err != nil {
			log.Fatal(err.Error()) // TODO
		}
		req.Header = request.Header
		*request = *req
	}

	rt := NewLimitedRoundTripper(http.DefaultTransport, rlimit.Rate{Count: 3, Duration: 1 * time.Second}, 2)

	rp := &httputil.ReverseProxy{Director: director, Transport: rt}
	server := http.Server{
		Addr:    ":9000",
		Handler: rp,
	}
	return server.ListenAndServe()
}
