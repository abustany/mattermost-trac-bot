package trac

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
)

type HTTPTransport struct {
	http.Transport
	Log bool
}

func logHeader(headers http.Header) {
	for name, values := range headers {
		for _, v := range values {
			log.Printf("%s: %s", name, v)
		}
	}
}

func (t *HTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.Log {
		return t.Transport.RoundTrip(req)
	}

	var reqBody []byte

	if req.Body != nil {
		reqBody, err := ioutil.ReadAll(req.Body)
		req.Body.Close()

		if err != nil {
			return nil, err
		}

		req.Body = ioutil.NopCloser(bytes.NewReader(reqBody))
	}

	log.Printf("HTTP --->")
	log.Printf("%s %s", req.Method, req.URL.String())
	logHeader(req.Header)

	if req.Body != nil {
		log.Printf("%s", string(reqBody))
	}

	res, err := http.DefaultTransport.RoundTrip(req)

	if err != nil {
		return nil, err
	}

	resBody, err := ioutil.ReadAll(res.Body)
	res.Body.Close()

	if err != nil {
		return nil, err
	}

	res.Body = ioutil.NopCloser(bytes.NewReader(resBody))

	log.Printf("<--- HTTP")
	log.Printf("%s", res.Status)
	logHeader(res.Header)
	log.Printf("%s", string(resBody))

	return res, err
}
