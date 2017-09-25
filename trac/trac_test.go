package trac

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
)

type TestServer struct {
	t             *testing.T
	steps         []func(req *http.Request) *http.Response
	currentStep   int
	authenticated bool
}

func (s *TestServer) Do(req *http.Request) (*http.Response, error) {
	if s.currentStep == len(s.steps) {
		s.t.Errorf("More requests than steps")
		return makeResponse(http.StatusInternalServerError, req), nil
	}

	res := s.steps[s.currentStep](req)
	s.currentStep++

	return res, nil
}

func testServer(t *testing.T) *TestServer {
	return &TestServer{
		t:             t,
		steps:         nil,
		currentStep:   0,
		authenticated: false,
	}
}

const testDomainNoPort = "127.0.0.1"
const testDomain = testDomainNoPort + ":1234"
const testPath = "/testPrefix"
const testUrl = "http://" + testDomain + testPath
const testUsername = "user"
const testPassword = "password"

func (s *TestServer) authenticate() {
	s.steps = append(s.steps, func(req *http.Request) *http.Response {
		const loginUrl = testUrl + "/login"
		const authHeader = "Basic dXNlcjpwYXNzd29yZA=="

		req.Body = nil

		if req.URL.String() != loginUrl {
			s.t.Errorf("Invalid login URL: %s", req.URL)
			return makeResponse(http.StatusInternalServerError, req)
		}

		auth := req.Header.Get("Authorization")

		var res *http.Response

		if auth != authHeader {
			res = makeResponse(http.StatusForbidden, req)
			s.t.Errorf("Invalid auth header ('%s', expected '%s')", auth, authHeader)
		} else {
			s.authenticated = true
			res = makeResponse(http.StatusOK, req)

			authCookie := &http.Cookie{
				Name:   "trac_auth",
				Value:  "dad21f2313322902e4d8a70fbe588244",
				Domain: testDomainNoPort,
				Path:   testPath,
			}

			res.Header = map[string][]string{
				"Set-Cookie": []string{authCookie.String()},
			}
		}

		return res
	})
}

func (s *TestServer) sendTicket() {
	s.steps = append(s.steps, func(req *http.Request) *http.Response {
		const ticketUrl = testUrl + "/ticket/33?format=csv"

		if req.URL.String() != ticketUrl {
			s.t.Errorf("Invalid ticket URL: %s", req.URL)
		}

		if !s.authenticated {
			return makeResponse(http.StatusForbidden, req)
		}

		const csv = `id,summary,reporter,owner,description,type,status,priority,milestone,component,version,resolution,keywords,cc
33,Test ticket,reporter,owner,description,type,status,priority,milestone,component,version,resolution,keywords,cc
`
		res := makeResponse(http.StatusOK, req)
		res.Body = ioutil.NopCloser(bytes.NewReader([]byte(csv)))

		return res
	})
}

func makeResponse(code int, req *http.Request) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d", code),
		StatusCode:    code,
		Body:          ioutil.NopCloser(bytes.NewReader([]byte{})),
		ContentLength: 0,
		Request:       req,
	}
}

func TestAuthenticateBasic(t *testing.T) {
	s := testServer(t)
	s.authenticate()
	client, err := NewWithHttpClient(testUrl, AuthBasic, false, s)

	if err != nil {
		t.Fatalf("Error while creating client: %s", err)
	}

	client.Authenticate(testUsername, testPassword)
}

func TestReauthenticate(t *testing.T) {
	s := testServer(t)
	s.authenticate()
	s.sendTicket()
	s.sendTicket()
	s.authenticate()
	s.sendTicket()

	client, err := NewWithHttpClient(testUrl, AuthBasic, false, s)

	if err != nil {
		t.Fatalf("Error while creating client: %s", err)
	}

	if err := client.Authenticate(testUsername, testPassword); err != nil {
		t.Errorf("Authenticate returned false")
	}

	if _, err := client.GetTicket("33"); err != nil {
		t.Errorf("GetTicket failed")
	}

	// Simulate deauthentication
	s.authenticated = false

	if _, err := client.GetTicket("33"); err != nil {
		t.Errorf("GetTicket failed")
	}
}
