package main

import (
	"bytes"
	"fmt"
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
	app.Version = "0.0.1-SNAPSHOT"
	app.Usage = "Rate limit HTTP proxy"

	app.Commands = []cli.Command{
		{
			Name:      "run",
			Usage:     "Run proxy",
			Action:    runProxy,
			ArgsUsage: "forwardURL",
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "port, p",
					Usage: "Listen `port` number",
					Value: 9000,
				},
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
func runProxy(c *cli.Context) (err error) {
	if c.NArg() != 1 {
		log.Fatalln("forwardURL must be specified.")
	}
	forwardURL, err := rlimit.ParseURL(c.Args().Get(0))

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
	port := c.Uint("port")
	addr := fmt.Sprintf(":%d", port)
	log.Println("Lisning on", addr)

	server := http.Server{
		Addr:    addr,
		Handler: rp,
	}
	return server.ListenAndServe()
}
