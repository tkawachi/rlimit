package main

import (
	"bytes"
	"gopkg.in/urfave/cli.v1"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
	"time"
)

type LimitedRoundTripper struct {
	ch       chan int
	inner    http.RoundTripper
	nWaiting uint32
}

func NewLimitedRoundTripper(rt http.RoundTripper) *LimitedRoundTripper {
	return &LimitedRoundTripper{
		ch:       make(chan int, 2),
		inner:    rt,
		nWaiting: 0,
	}
}

func (lrt *LimitedRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {

	tooManyRequestsThreshold := uint32(2)

	waiting := atomic.LoadUint32(&lrt.nWaiting)
	if waiting > tooManyRequestsThreshold {
		if req.Body != nil {
			err = req.Body.Close()
		}
		resp = &http.Response{
			// Request: req,
			StatusCode: 429,
			Status:     "Too Many Requests",
			Body:       ioutil.NopCloser(bytes.NewBufferString("Too Many Requests (rlimit)\n")),
		}
		return
	}
	someInt := 0
	duration := 3 * time.Second

	atomic.AddUint32(&lrt.nWaiting, 1)
	lrt.ch <- someInt
	atomic.AddUint32(&lrt.nWaiting, ^uint32(0)) // decrement
	defer func() {
		go func() {
			time.Sleep(duration)
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

	forwardUrl, err := url.Parse(c.String("forward"))
	if err != nil {
		return
	}

	director := func(request *http.Request) {
		url := *request.URL
		url.Scheme = forwardUrl.Scheme
		url.Host = forwardUrl.Host

		req, err := http.NewRequest(request.Method, url.String(), request.Body)
		if err != nil {
			log.Fatal(err.Error()) // TODO
		}
		req.Header = request.Header
		*request = *req
	}

	rt := NewLimitedRoundTripper(http.DefaultTransport)

	rp := &httputil.ReverseProxy{Director: director, Transport: rt}
	server := http.Server{
		Addr:    ":9000",
		Handler: rp,
	}
	return server.ListenAndServe()
}
