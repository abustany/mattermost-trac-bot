package trac

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Functions below copied from Go's net/http

func httpGet(client HttpClient, url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

func httpPostForm(client HttpClient, url string, data url.Values) (resp *http.Response, err error) {

	return httpPost(client, url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))

}

func httpPost(client HttpClient, url string, contentType string, body io.Reader) (resp *http.Response, err error) {

	req, err := http.NewRequest("POST", url, body)

	if err != nil {

		return nil, err

	}

	req.Header.Set("Content-Type", contentType)

	return client.Do(req)

}
