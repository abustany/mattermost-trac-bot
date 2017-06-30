package trac

import (
	"bytes"
	"crypto/tls"
	"encoding/csv"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/publicsuffix"
)

type AuthType uint

const (
	AuthBasic AuthType = iota
	AuthForm
)

type Client struct {
	url      string
	authType AuthType
	client   *http.Client
}

// Ticket in Trac can come in any shape, so our representation is just a map of
// strings. There will always be a "_url" member in the hash being the URL to
// the ticket, the other fields depend of the Trac configuration.
type Ticket map[string]string

func ParseAuthType(s string) (AuthType, error) {
	s = strings.ToLower(s)

	switch s {
	case "basic":
		return AuthBasic, nil
	case "form":
		return AuthForm, nil
	default:
		return AuthBasic, errors.Errorf("Invalid AuthType string: %s", s)
	}
}

func NewClient(url string, authType AuthType) (*Client, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})

	if err != nil {
		return nil, errors.Wrap(err, "Error while initializing public suffix list")
	}

	return &Client{
		url:      url,
		authType: authType,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func (c *Client) SetInsecure(insecure bool) {
	if insecure {
		c.client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	} else {
		c.client.Transport = nil
	}
}

func (c *Client) authenticateBasic(username, password string) error {
	req, err := http.NewRequest("GET", c.url+"/login", nil)

	if err != nil {
		return errors.Wrap(err, "Error while initializing request")
	}

	req.SetBasicAuth(username, password)

	resp, err := c.client.Do(req)

	if err != nil {
		return errors.Wrap(err, "Error while sending login request")
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound {
		return nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("Invalid username or password")
	}

	return errors.Errorf("Unexpected HTTP status: %d", resp.StatusCode)
}

var TOKEN_FORM_RE = regexp.MustCompile(`<input\s+type="hidden"\s+name="__FORM_TOKEN"\s+value="([a-z0-9]+)"\s+/>`)

func (c *Client) getLoginFormToken() (string, error) {
	resp, err := c.client.Get(c.url + "/login")

	if err != nil {
		return "", errors.Wrap(err, "Error while retrieving login page")
	}

	defer resp.Body.Close()

	loginHtml, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return "", errors.Wrap(err, "Error while reading login page")
	}

	// It seems we could also retrieve the value from the cookies
	match := TOKEN_FORM_RE.FindSubmatch(loginHtml)

	if match == nil {
		return "", errors.New("Cannot find form token in login page")
	}

	return string(match[1]), nil
}

func (c *Client) authenticateForm(username, password string) error {
	// First get the login page to get the form token
	formToken, err := c.getLoginFormToken()

	if err != nil {
		return errors.Wrap(err, "Error while loading form token")
	}

	resp, err := c.client.PostForm(c.url+"/login", url.Values{
		"user":         {username},
		"password":     {password},
		"referer":      {c.url},
		"__FORM_TOKEN": {formToken},
	})

	if err != nil {
		return errors.Wrap(err, "Error while sending login request")
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound {
		return nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("Invalid username or password")
	}

	return errors.Errorf("Unexpected HTTP status: %d", resp.StatusCode)
}

func (c *Client) Authenticate(username, password string) error {
	switch c.authType {
	case AuthBasic:
		return c.authenticateBasic(username, password)
	case AuthForm:
		return c.authenticateForm(username, password)
	default:
		panic("Unknown auth type")
	}
}

func (c *Client) GetTicket(id string) (Ticket, error) {
	ticketUrl := c.url + "/ticket/" + id
	csvTicketUrl := ticketUrl + "?format=csv"

	log.Printf("GET %s", csvTicketUrl)

	resp, err := c.client.Get(csvTicketUrl)

	if err != nil {
		return Ticket{}, errors.Wrap(err, "Error while sending ticket request")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Ticket{}, errors.Errorf("Unexpected HTTP status: %d", resp.StatusCode)
	}

	csvData, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return Ticket{}, errors.Wrap(err, "Error while reading response data")
	}

	// Trac seems to send a UTF8 BOM, strip it if present
	if bytes.HasPrefix(csvData, []byte{0xef, 0xbb, 0xbf}) {
		csvData = csvData[3:]
	}

	records, err := csv.NewReader(bytes.NewReader(csvData)).ReadAll()

	if err != nil {
		return Ticket{}, errors.Wrap(err, "Error while decoding CSV")
	}

	if len(records) != 2 || len(records[0]) != len(records[1]) {
		return Ticket{}, errors.New("Unexpected number of records in CSV")
	}

	ticket := map[string]string{}

	for idx, field := range records[0] {
		ticket[field] = records[1][idx]
	}

	ticket["_url"] = ticketUrl

	return ticket, nil
}
